package main

import (
	"flag"
	"os"

	"github.com/go-logr/zapr"
	autoscalerv1alpha1 "github.com/YOUR_GITHUB_USERNAME/prometheus-policy-autoscaler/api/v1alpha1"
	autoscalercontroller "github.com/YOUR_GITHUB_USERNAME/prometheus-policy-autoscaler/pkg/controller"
	"github.com/YOUR_GITHUB_USERNAME/prometheus-policy-autoscaler/pkg/history"
	"github.com/YOUR_GITHUB_USERNAME/prometheus-policy-autoscaler/pkg/metrics"
	"github.com/YOUR_GITHUB_USERNAME/prometheus-policy-autoscaler/pkg/policy"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	// Register built-in Kubernetes types.
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// Register our custom PrometheusAutoscaler API.
	utilruntime.Must(autoscalerv1alpha1.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr          string
		probeAddr            string
		enableLeaderElection bool
		logLevel             string
	)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "Address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true, "Enable leader election for controller manager.")
	flag.StringVar(&logLevel, "log-level", "info", "Log level: debug|info|warn|error")
	flag.Parse()

	// Configure a structured JSON logger for production use.
	zapCfg := zap.NewProductionConfig()
	zapCfg.Encoding = "json"

	switch logLevel {
	case "debug":
		zapCfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	case "warn":
		zapCfg.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
	case "error":
		zapCfg.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	default:
		zapCfg.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}

	zapLog, err := zapCfg.Build()
	if err != nil {
		panic(err)
	}
	defer zapLog.Sync()

	ctrl.SetLogger(zapr.NewLogger(zapLog))

	// Use in-cluster config when running inside Kubernetes.
	cfg := ctrl.GetConfigOrDie()

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "prometheus-policy-autoscaler.parspack.dev",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Shared dependencies for the reconciler.
	historyStore := history.NewStore()
	engine := policy.NewEngine()

	reconciler := &autoscalercontroller.PrometheusAutoscalerReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("prometheus-policy-autoscaler"),
		Logger:   ctrl.Log.WithName("controller").WithName("PrometheusAutoscaler"),

		PromClientFactory: func(url string) (metrics.Client, error) {
			// This factory keeps the reconciler decoupled from concrete implementations.
			return metrics.NewHTTPClient(url)
		},
		PolicyEngine: engine,
		HistoryStore: historyStore,
	}

	if err := reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PrometheusAutoscaler")
		os.Exit(1)
	}

	// Health/readiness probes for Kubernetes.
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
