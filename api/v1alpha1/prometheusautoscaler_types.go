package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=prometheusautoscalers,scope=Namespaced,shortName=pas
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.targetRef.name`
// +kubebuilder:printcolumn:name="Min",type=integer,JSONPath=`.spec.minReplicas`
// +kubebuilder:printcolumn:name="Max",type=integer,JSONPath=`.spec.maxReplicas`
// +kubebuilder:printcolumn:name="Current",type=integer,JSONPath=`.status.currentReplicas`
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=`.status.desiredReplicas`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Mode defines how the controller should act on this autoscaler.
type Mode string

const (
    ModeApply  Mode = "Apply"
    ModeDryRun Mode = "DryRun"
)

// PrometheusConfig configures how we talk to Prometheus for this autoscaler.
type PrometheusConfig struct {
    // URL is the base URL of the Prometheus HTTP API.
    // Example: http://prometheus.monitoring.svc.cluster.local:9090
    // +kubebuilder:validation:MinLength=1
    URL string `json:"url"`

    // AuthSecretRef optionally points to a Secret containing auth details.
    // Keeping credentials out of the CR spec helps avoid accidental leaks.
    // +optional
    AuthSecretRef *string `json:"authSecretRef,omitempty"`
}

// ScaleDirection defines thresholds and step sizes for scaling decisions.
type ScaleDirection struct {
    // Threshold defines the metric value at which we start scaling.
    // For scaleUp, values higher than threshold trigger scaling.
    // For scaleDown, values lower than threshold trigger scaling.
    Threshold float64 `json:"threshold"`

    // Step defines how many replicas we change in one decision.
    // We apply further rate limiting in the policy engine.
    // +kubebuilder:validation:Minimum=0
    Step int32 `json:"step"`
}

// MetricSpec describes one PromQL-based signal used to drive scaling.
type MetricSpec struct {
    // Name is a logical name for this metric within the policy.
    Name string `json:"name"`

    // PromQL is the query we send to Prometheus.
    // Ideally it evaluates to a single scalar or a single-element vector.
    PromQL string `json:"promQL"`

    // Weight is used when aggregation=weighted to combine decisions.
    // +optional
    // +kubebuilder:validation:Minimum=0
    Weight *float64 `json:"weight,omitempty"`

    // ScaleUp defines how we behave when the metric is above a threshold.
    // +optional
    ScaleUp *ScaleDirection `json:"scaleUp,omitempty"`

    // ScaleDown defines how we behave when the metric is below a threshold.
    // +optional
    ScaleDown *ScaleDirection `json:"scaleDown,omitempty"`
}

// AggregationStrategy defines how we combine per-metric desired replicas.
type AggregationStrategy string

const (
    AggregationMax      AggregationStrategy = "max"
    AggregationMin      AggregationStrategy = "min"
    AggregationAverage  AggregationStrategy = "average"
    AggregationWeighted AggregationStrategy = "weighted"
)

// BehaviorSpec configures stabilization, cooldown and rate limiting.
type BehaviorSpec struct {
    // StabilizationWindowSeconds defines how long we remember past desired
    // values to avoid flapping, especially on scale-down.
    // +optional
    // +kubebuilder:validation:Minimum=0
    StabilizationWindowSeconds *int32 `json:"stabilizationWindowSeconds,omitempty"`

    // ScaleUpCooldownSeconds prevents back-to-back scale-ups too quickly.
    // +optional
    // +kubebuilder:validation:Minimum=0
    ScaleUpCooldownSeconds *int32 `json:"scaleUpCooldownSeconds,omitempty"`

    // ScaleDownCooldownSeconds prevents back-to-back scale-downs too quickly.
    // +optional
    // +kubebuilder:validation:Minimum=0
    ScaleDownCooldownSeconds *int32 `json:"scaleDownCooldownSeconds,omitempty"`

    // MaxScaleUpStepPercent limits how much we can grow in a single decision.
    // +optional
    // +kubebuilder:validation:Minimum=0
    MaxScaleUpStepPercent *int32 `json:"maxScaleUpStepPercent,omitempty"`

    // MaxScaleDownStepPercent limits how much we can shrink in a single decision.
    // +optional
    // +kubebuilder:validation:Minimum=0
    MaxScaleDownStepPercent *int32 `json:"maxScaleDownStepPercent,omitempty"`
}

// TargetRef points to the workload we want to scale.
type TargetRef struct {
    APIVersion string `json:"apiVersion"`
    Kind       string `json:"kind"`
    Name       string `json:"name"`
    Namespace  string `json:"namespace"`
}

// PrometheusAutoscalerSpec defines the desired behavior for one autoscaler.
type PrometheusAutoscalerSpec struct {
    TargetRef TargetRef `json:"targetRef"`

    // MinReplicas is the lower bound of the replica count.
    // +kubebuilder:validation:Minimum=1
    MinReplicas int32 `json:"minReplicas"`

    // MaxReplicas is the upper bound of the replica count.
    // +kubebuilder:validation:Minimum=1
    MaxReplicas int32 `json:"maxReplicas"`

    // Mode allows users to run in DryRun to see what the controller
    // would do without actually changing the target workload.
    // +kubebuilder:validation:Enum=Apply;DryRun
    // +optional
    Mode Mode `json:"mode,omitempty"`

    // Prometheus configuration for this autoscaler.
    Prometheus PrometheusConfig `json:"prometheus"`

    // Aggregation tells the policy engine how to combine per-metric decisions.
    // +kubebuilder:validation:Enum=max;min;average;weighted
    // +optional
    Aggregation AggregationStrategy `json:"aggregation,omitempty"`

    // Metrics is the list of PromQL-based signals that drive scaling.
    Metrics []MetricSpec `json:"metrics"`

    // Behavior defines stabilization, cooldown and rate limiting knobs.
    // +optional
    Behavior *BehaviorSpec `json:"behavior,omitempty"`
}

// PrometheusAutoscalerStatus captures what the controller last computed/applied.
type PrometheusAutoscalerStatus struct {
    // CurrentReplicas is what we see on the target workload right now.
    // +optional
    CurrentReplicas *int32 `json:"currentReplicas,omitempty"`

    // DesiredReplicas is what the policy engine last computed.
    // +optional
    DesiredReplicas *int32 `json:"desiredReplicas,omitempty"`

    // LastScaleTime is when we last changed the replica count.
    // +optional
    LastScaleTime *metav1.Time `json:"lastScaleTime,omitempty"`

    // LastPrometheusSample is a JSON string summarizing metrics used
    // in the last decision. Helps with debugging.
    // +optional
    LastPrometheusSample string `json:"lastPrometheusSample,omitempty"`

    // Conditions follows the standard Kubernetes pattern to surface health.
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// PrometheusAutoscaler is the Schema for the autoscalers API.
// +kubebuilder:object:root=true
type PrometheusAutoscaler struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   PrometheusAutoscalerSpec   `json:"spec,omitempty"`
    Status PrometheusAutoscalerStatus `json:"status,omitempty"`
}

// PrometheusAutoscalerList contains a list of PrometheusAutoscaler.
// +kubebuilder:object:root=true
type PrometheusAutoscalerList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []PrometheusAutoscaler `json:"items"`
}

func init() {
    // Register our types with the scheme so Kubernetes knows about them.
    SchemeBuilder.Register(&PrometheusAutoscaler{}, &PrometheusAutoscalerList{})
}
