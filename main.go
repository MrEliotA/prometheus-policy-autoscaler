package main

import (
    "flag"
    "os"

    "github.com/go-logr/zapr"
    autoscalerv1alpha1 "github.com/MreliotA/prometheus-policy-autoscaler/api/v1alpha1"
    "github.com/MreliotA/prometheus-policy-autoscaler/controllers"
    "github.com/MreliotA/prometheus-policy-autoscaler/pkg/history"
    "github.com/MreliotA/prometheus-policy-autoscaler/pkg/metrics"
    "github.com/MreliotA/prometheus-policy-autoscaler/pkg/policy"
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
    // Register built-in Kubernetes types into the scheme.
    utilruntime.Must(clientgoscheme.AddToScheme(scheme))

    // Register our custom API (PrometheusAutoscaler) into the scheme.
    utilruntime.Must(autoscalerv1alpha1.AddToScheme(scheme))
}

func main() {
    var (
        metricsAddr          string
        probeAddr            string
        enableLeaderElection bool
        logLevel             string
    )

    flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "Address the metrics endpoint binds to.")
    flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Address the health probe endpoint binds to.")
    flag.BoolVar(&enableLeaderElection, "leader-elect", false,
        "Enable leader election for controller manager. Ensures only one active instance.")
    flag.StringVar(&logLevel, "log-level", "info", "Log level: debug|info|warn|error")
    flag.Parse()

    // Configure a structured JSON logger. This is production-friendly and plays
    // nicely with most logging stacks (ELK, Loki, etc).
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

    cfg := ctrl.GetConfigOrDie()

    mgr, err := ctrl.NewManager(cfg, ctrl.Options{
        Scheme:                 scheme,
        MetricsBindAddress:     metricsAddr,
        HealthProbeBindAddress: probeAddr,
        LeaderElection:         enableLeaderElection,
        LeaderElectionID:       "prometheus-autoscaler.parspack.dev",
    })
    if err != nil {
        setupLog.Error(err, "unable to start manager")
        os.Exit(1)
    }

    // Shared dependencies for the reconciler.
    historyStore := history.NewStore()
    engine := policy.NewEngine()

    reconciler := &controllers.PrometheusAutoscalerReconciler{
        Client: mgr.GetClient(),
        Scheme: mgr.GetScheme(),
        Recorder: mgr.GetEventRecorderFor("prometheus-autoscaler"),
        Logger:   ctrl.Log.WithName("controller").WithName("PrometheusAutoscaler"),

        PromClientFactory: func(url string) (metrics.Client, error) {
            // In a real setup, you might want to cache these clients per URL
            // instead of creating new ones on each reconciliation.
            return metrics.NewHTTPClient(url)
        },
        PolicyEngine: engine,
        HistoryStore: historyStore,
    }

    if err := reconciler.SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "PrometheusAutoscaler")
        os.Exit(1)
    }

    // Health and readiness probes so Kubernetes can monitor our controller.
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
