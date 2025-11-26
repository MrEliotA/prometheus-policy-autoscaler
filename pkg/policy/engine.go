package policy

import (
    "fmt"
    "time"

    autoscalerv1alpha1 "github.com/MreliotA/prometheus-policy-autoscaler/api/v1alpha1"
)

// Input is what the reconciler passes into the engine for a single decision.
type Input struct {
    CurrentReplicas int32
    Spec            autoscalerv1alpha1.PrometheusAutoscalerSpec

    // Samples maps metric name -> last Prometheus value.
    Samples map[string]float64

    // Meta information for behavior tuning.
    Now           time.Time
    LastScaleTime *time.Time

    // History carries past desired values to implement stabilization windows.
    History []HistorySample
}

// HistorySample is a lightweight record we store per evaluation.
type HistorySample struct {
    Timestamp       time.Time
    DesiredReplicas int32
}

// Decision is the engine's answer for one reconciliation tick.
type Decision struct {
    DesiredReplicas int32
    Reason          string
    CooldownActive  bool
}

// Engine defines the contract; keeping it as an interface allows easy testing
// and future alternative implementations (e.g. more advanced strategies).
type Engine interface {
    Decide(input Input) (Decision, error)
}

// DefaultEngine is a straightforward implementation tuned for readability.
type DefaultEngine struct{}

// NewEngine returns a new DefaultEngine instance.
func NewEngine() Engine {
    return &DefaultEngine{}
}

// Decide implements the core policy logic for one reconciliation cycle.
func (e *DefaultEngine) Decide(in Input) (Decision, error) {
    if len(in.Spec.Metrics) == 0 {
        return Decision{}, fmt.Errorf("no metrics defined in spec")
    }

    desired := in.CurrentReplicas
    reasons := []string{}

    var metricDesired []int32
    var metricWeights []float64

    for _, ms := range in.Spec.Metrics {
        sample, ok := in.Samples[ms.Name]
        if !ok {
            // Missing metric is treated as neutral. We log this at the call site
            // instead of failing the entire reconciliation.
            metricDesired = append(metricDesired, desired)
            metricWeights = append(metricWeights, 1.0)
            continue
        }

        perMetricDesired := e.desiredFromMetric(in.CurrentReplicas, sample, ms)
        metricDesired = append(metricDesired, perMetricDesired)

        weight := 1.0
        if ms.Weight != nil {
            weight = *ms.Weight
        }
        metricWeights = append(metricWeights, weight)

        reasons = append(reasons, fmt.Sprintf("%s=%.4f -> %d", ms.Name, sample, perMetricDesired))
    }

    aggregated := e.aggregate(metricDesired, metricWeights, in.Spec.Aggregation)
    desired = aggregated

    // Respect hard min/max bounds from spec.
    if desired < in.Spec.MinReplicas {
        desired = in.Spec.MinReplicas
    }
    if desired > in.Spec.MaxReplicas {
        desired = in.Spec.MaxReplicas
    }

    cooled, cooldownActive := e.applyCooldownAndHistory(in, desired)
    desired = cooled

    reason := fmt.Sprintf("metrics=[%s], aggregation=%s", joinReasons(reasons), in.Spec.Aggregation)

    return Decision{
        DesiredReplicas: desired,
        Reason:          reason,
        CooldownActive:  cooldownActive,
    }, nil
}

// desiredFromMetric maps one metric sample to a desired replica count.
func (e *DefaultEngine) desiredFromMetric(current int32, sample float64, ms autoscalerv1alpha1.MetricSpec) int32 {
    desired := current

    // Scale up if configured and sample is above the threshold.
    if ms.ScaleUp != nil && sample > ms.ScaleUp.Threshold {
        desired = current + ms.ScaleUp.Step
    }

    // Scale down if configured and sample is below the threshold.
    if ms.ScaleDown != nil && sample < ms.ScaleDown.Threshold {
        desired = current - ms.ScaleDown.Step
        if desired < 1 {
            desired = 1
        }
    }

    return desired
}

// aggregate combines the per-metric recommendations into a single number.
func (e *DefaultEngine) aggregate(desired []int32, weights []float64, strategy autoscalerv1alpha1.AggregationStrategy) int32 {
    if len(desired) == 0 {
        return 1
    }

    switch strategy {
    case autoscalerv1alpha1.AggregationMin:
        min := desired[0]
        for _, v := range desired[1:] {
            if v < min {
                min = v
            }
        }
        return min
    case autoscalerv1alpha1.AggregationAverage:
        var sum int32
        for _, v := range desired {
            sum += v
        }
        return sum / int32(len(desired))
    case autoscalerv1alpha1.AggregationWeighted:
        var num, den float64
        for i, v := range desired {
            w := weights[i]
            num += float64(v) * w
            den += w
        }
        if den == 0 {
            return desired[0]
        }
        return int32(num / den)
    case autoscalerv1alpha1.AggregationMax:
        fallthrough
    default:
        max := desired[0]
        for _, v := range desired[1:] {
            if v > max {
                max = v
            }
        }
        return max
    }
}

// applyCooldownAndHistory limits how aggressively we apply desired changes.
func (e *DefaultEngine) applyCooldownAndHistory(in Input, desired int32) (int32, bool) {
    behavior := in.Spec.Behavior
    if behavior == nil {
        return desired, false
    }

    cooldownActive := false

    if in.LastScaleTime != nil {
        elapsed := in.Now.Sub(*in.LastScaleTime)

        if desired > in.CurrentReplicas && behavior.ScaleUpCooldownSeconds != nil {
            if elapsed < time.Duration(*behavior.ScaleUpCooldownSeconds)*time.Second {
                cooldownActive = true
                desired = in.CurrentReplicas
            }
        }

        if desired < in.CurrentReplicas && behavior.ScaleDownCooldownSeconds != nil {
            if elapsed < time.Duration(*behavior.ScaleDownCooldownSeconds)*time.Second {
                cooldownActive = true
                desired = in.CurrentReplicas
            }
        }
    }

    // Stabilization window: for scale-down, consider the max desired in the window
    // to avoid flapping due to transient dips.
    if behavior.StabilizationWindowSeconds != nil && desired < in.CurrentReplicas {
        window := time.Duration(*behavior.StabilizationWindowSeconds) * time.Second
        cutoff := in.Now.Add(-window)

        maxInWindow := desired
        for _, h := range in.History {
            if h.Timestamp.After(cutoff) && h.DesiredReplicas > maxInWindow {
                maxInWindow = h.DesiredReplicas
            }
        }
        desired = maxInWindow
    }

    delta := desired - in.CurrentReplicas
    if delta == 0 {
        return desired, cooldownActive
    }

    // Rate limiting: cap how much we can change in one reconciliation.
    if delta > 0 && behavior.MaxScaleUpStepPercent != nil {
        allowed := rateLimitStep(in.CurrentReplicas, *behavior.MaxScaleUpStepPercent)
        if delta > allowed {
            desired = in.CurrentReplicas + allowed
        }
    }

    if delta < 0 && behavior.MaxScaleDownStepPercent != nil {
        allowed := rateLimitStep(in.CurrentReplicas, *behavior.MaxScaleDownStepPercent)
        if -delta > allowed {
            desired = in.CurrentReplicas - allowed
        }
    }

    if desired < 1 {
        desired = 1
    }

    return desired, cooldownActive
}

// rateLimitStep returns the absolute step size allowed for a given percentage.
func rateLimitStep(current int32, percent int32) int32 {
    if percent <= 0 {
        return 0
    }
    step := (current * percent) / 100
    if step < 1 {
        step = 1
    }
    return step
}

func joinReasons(parts []string) string {
    if len(parts) == 0 {
        return ""
    }
    out := parts[0]
    for _, p := range parts[1:] {
        out += "; " + p
    }
    return out
}
