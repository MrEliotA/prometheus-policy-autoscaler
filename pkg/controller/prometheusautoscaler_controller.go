package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	autoscalerv1alpha1 "github.com/MreliotA/prometheus-policy-autoscaler/api/v1alpha1"
	"github.com/MreliotA/prometheus-policy-autoscaler/pkg/history"
	"github.com/MreliotA/prometheus-policy-autoscaler/pkg/metrics"
	"github.com/MreliotA/prometheus-policy-autoscaler/pkg/policy"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NOTE: These RBAC markers are used by controller-gen to generate RBAC manifests.
// They do not affect runtime behavior but are very useful when you automate RBAC.
// +kubebuilder:rbac:groups=autoscaler.laravel.app,resources=prometheusautoscalers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=autoscaler.laravel.app,resources=prometheusautoscalers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// PrometheusAutoscalerReconciler reconciles a PrometheusAutoscaler object.
type PrometheusAutoscalerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Logger   logr.Logger

	PromClientFactory func(url string) (metrics.Client, error)
	PolicyEngine      policy.Engine
	HistoryStore      *history.Store
}

// Reconcile implements the core control loop for PrometheusAutoscaler.
// It pulls Prometheus metrics, runs the policy engine, and patches
// the target Deployment's replica count accordingly.
func (r *PrometheusAutoscalerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Logger.WithValues("prometheusautoscaler", req.NamespacedName)

	var pa autoscalerv1alpha1.PrometheusAutoscaler
	if err := r.Get(ctx, req.NamespacedName, &pa); err != nil {
		if apierrors.IsNotFound(err) {
			// The CR was deleted; nothing left to do.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting PrometheusAutoscaler: %w", err)
	}

	if !pa.ObjectMeta.DeletionTimestamp.IsZero() {
		// We don't implement any finalizer logic yet.
		return ctrl.Result{}, nil
	}

	if pa.Status.Conditions == nil {
		pa.Status.Conditions = []metav1.Condition{}
	}

	targetKey := types.NamespacedName{
		Namespace: pa.Spec.TargetRef.Namespace,
		Name:      pa.Spec.TargetRef.Name,
	}

	var deploy appsv1.Deployment
	if err := r.Get(ctx, targetKey, &deploy); err != nil {
		if apierrors.IsNotFound(err) {
			r.setCondition(&pa, "TargetFound", metav1.ConditionFalse, "NotFound",
				"Target workload %s/%s not found", targetKey.Namespace, targetKey.Name)
			_ = r.Status().Update(ctx, &pa)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting target deployment: %w", err)
	}

	promClient, err := r.PromClientFactory(pa.Spec.Prometheus.URL)
	if err != nil {
		r.setCondition(&pa, "PrometheusAvailable", metav1.ConditionFalse, "ClientError", err.Error())
		_ = r.Status().Update(ctx, &pa)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	samples := make(map[string]float64)
	for _, ms := range pa.Spec.Metrics {
		val, err := promClient.QueryVector(ctx, ms.PromQL)
		if err != nil {
			log.Error(err, "failed to query Prometheus", "metric", ms.Name, "promql", ms.PromQL)
			r.setCondition(&pa, "PrometheusAvailable", metav1.ConditionFalse, "QueryError", err.Error())
			_ = r.Status().Update(ctx, &pa)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		samples[ms.Name] = val
	}

	currentReplicas := int32(1)
	if deploy.Spec.Replicas != nil {
		currentReplicas = *deploy.Spec.Replicas
	}

	var lastScaleTime *time.Time
	if pa.Status.LastScaleTime != nil {
		t := pa.Status.LastScaleTime.Time
		lastScaleTime = &t
	}

	historyKey := fmt.Sprintf("%s/%s", pa.Namespace, pa.Name)
	hist := r.HistoryStore.Get(historyKey)

	input := policy.Input{
		CurrentReplicas: currentReplicas,
		Spec:            pa.Spec,
		Samples:         samples,
		Now:             time.Now(),
		LastScaleTime:   lastScaleTime,
		History:         hist,
	}

	decision, err := r.PolicyEngine.Decide(input)
	if err != nil {
		log.Error(err, "policy engine failed")
		r.setCondition(&pa, "SpecValid", metav1.ConditionFalse, "PolicyError", err.Error())
		_ = r.Status().Update(ctx, &pa)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	desired := decision.DesiredReplicas

	// Update in-memory history with the latest decision.
	r.HistoryStore.Append(historyKey, policy.HistorySample{
		Timestamp:       input.Now,
		DesiredReplicas: desired,
	}, 20)

	sampleJSON, _ := json.Marshal(samples)
	pa.Status.LastPrometheusSample = string(sampleJSON)
	pa.Status.DesiredReplicas = &desired
	pa.Status.CurrentReplicas = &currentReplicas

	// DryRun mode: compute decisions but do not touch the target Deployment.
	if pa.Spec.Mode == autoscalerv1alpha1.ModeDryRun {
		log.Info("dry-run mode: not applying scaling", "current", currentReplicas, "desired", desired)
		r.setCondition(&pa, "Ready", metav1.ConditionTrue, "DryRun",
			"Computed desired replicas in DryRun mode; no changes applied")
		if err := r.Status().Update(ctx, &pa); err != nil {
			log.Error(err, "failed to update status in dry-run mode")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// If nothing changed, just refresh status and requeue later.
	if desired == currentReplicas {
		log.Info("no scaling required", "replicas", currentReplicas)
		r.setCondition(&pa, "Ready", metav1.ConditionTrue, "SteadyState",
			"Current replicas already match desired")
		if err := r.Status().Update(ctx, &pa); err != nil {
			log.Error(err, "failed to update status in steady state")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Patch target Deployment with the new replica count.
	patched := deploy.DeepCopy()
	patched.Spec.Replicas = &desired

	if err := r.Patch(ctx, patched, client.MergeFrom(&deploy)); err != nil {
		log.Error(err, "failed to patch target deployment", "desired", desired)
		r.setCondition(&pa, "Ready", metav1.ConditionFalse, "ScaleFailed", err.Error())
		_ = r.Status().Update(ctx, &pa)
		return ctrl.Result{}, fmt.Errorf("patching target deployment: %w", err)
	}

	now := metav1.Now()
	pa.Status.LastScaleTime = &now
	r.setCondition(&pa, "Ready", metav1.ConditionTrue, "Scaled",
		"Scaled from %d to %d", currentReplicas, desired)
	if err := r.Status().Update(ctx, &pa); err != nil {
		log.Error(err, "failed to update status after scaling")
	}

	// Emit a Kubernetes Event for easy tracing in kubectl describe.
	r.Recorder.Eventf(&pa, "Normal", "Scaled",
		"Scaled target %s/%s from %d to %d",
		targetKey.Namespace, targetKey.Name, currentReplicas, desired)

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// setCondition is a small helper to keep condition updates consistent.
func (r *PrometheusAutoscalerReconciler) setCondition(
	pa *autoscalerv1alpha1.PrometheusAutoscaler,
	condType string,
	status metav1.ConditionStatus,
	reason, msgFmt string,
	args ...interface{},
) {
	msg := fmt.Sprintf(msgFmt, args...)
	metav1.SetStatusCondition(&pa.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		LastTransitionTime: metav1.Now(),
	})
}

// SetupWithManager wires this reconciler into the controller-runtime manager.
func (r *PrometheusAutoscalerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalerv1alpha1.PrometheusAutoscaler{}).
		Complete(r)
}
