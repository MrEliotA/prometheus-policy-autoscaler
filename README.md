Prometheus Policy Autoscaler

Prometheus-driven Kubernetes autoscaler for Laravel-based workloads (web + queues), with policy-as-code, PromQL, and a GitOps-friendly CI/CD pipeline (Jenkins + Helm + Argo CD).

A controller that understands your application and database metrics – not just CPU – and scales based on real Laravel traffic, queue backlog, MySQL, and Redis pressure.

Table of Contents

Motivation

Key Features

High-Level Architecture

Custom Resource: PrometheusAutoscaler

Laravel Metrics & PromQL Signals

CI/CD: Jenkins + Helm + Argo CD

Repository Layout

Getting Started

Deploying the Controller

Deploying the Laravel Demo Stack

Extending to Other Workloads

Next Steps

Motivation

Horizontal Pod Autoscalers (HPA) are great, but they tend to focus on resource metrics (CPU, memory) or a single custom metric. Real applications – especially Laravel monoliths with queues, MySQL, and Redis – need more context:

How many HTTP requests per second are hitting the app?

What is the p95 latency for critical endpoints?

Are queue jobs backing up?

Is MySQL close to max_connections?

Is Redis running hot or near its memory limit?

This project implements a Kubernetes controller in Go that:

Defines a CRD (PrometheusAutoscaler) where autoscaling logic is expressed as PromQL policies.

Targets Laravel workloads (web + queue workers) as a demo, but is generic enough for other apps.

Uses Prometheus as the single source of truth for metrics.

Is deployed via Helm and managed by Argo CD in a GitOps workflow.

Is built, tested, and released through Jenkins CI.

Key Features

Custom Resource Definition: PrometheusAutoscaler in API group autoscaler.laravel.app/v1alpha1.

Policy-as-code:

Multiple metrics per policy.

Per-metric scaleUp / scaleDown thresholds and steps.

Aggregation strategies: max, min, average, weighted.

Behavior tuning: stabilization windows, cooldowns, rate limits.

Laravel-aware autoscaling:

HTTP RPS, latency, error rate.

Queue backlog and oldest job age (Horizon/queue metrics).

MySQL threads / connections via mysqld_exporter.

Redis memory usage via redis_exporter.

Dry-run mode:

See what would happen without actually scaling.

Controller-runtime based:

Written in Go using sigs.k8s.io/controller-runtime.

Uses structured logging, health/readiness probes, and Kubernetes events.

GitOps-friendly:

Controller deployed via Helm chart.

Argo CD watches the Git repository and syncs changes.

Jenkins builds/pushes images and updates Helm values.yaml with new image tags.

High-Level Architecture

Control loop:

PrometheusAutoscaler objects are created in the cluster.

The Go controller watches these CRs.

For each CR:

Fetch the referenced Deployment (Laravel web / worker).

Query Prometheus with the configured PromQL expressions.

Feed samples into the policy engine.

The engine computes desiredReplicas based on:

Per-metric thresholds & steps.

Aggregation strategy.

Stabilization / cooldown / rate limits.

If not in DryRun and desiredReplicas != currentReplicas, patch the Deployment.

Status is updated with:

currentReplicas

desiredReplicas

lastScaleTime

a JSON snapshot of the last metrics used.

CI/CD flow:

Developer pushes code to GitHub.

Jenkins pipeline:

Runs go test ./....

Builds Docker image from Dockerfile.

Tags image with Git commit SHA.

Pushes image to registry.

Updates deploy/helm/prometheus-autoscaler/values.yaml (image.tag) via yq.

Commits and pushes back to Git.

Argo CD:

Detects the chart change.

Syncs the updated controller to Kubernetes.

Similarly, Argo CD manages the Laravel demo stack via its own Helm chart.

Custom Resource: PrometheusAutoscaler

The CR lives in the group autoscaler.laravel.app and version v1alpha1.

Spec (simplified)
apiVersion: autoscaler.laravel.app/v1alpha1
kind: PrometheusAutoscaler
metadata:
  name: laravel-web-autoscaler
  namespace: laravel-demo
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: laravel-web
    namespace: laravel-demo

  minReplicas: 3
  maxReplicas: 30
  mode: Apply   # or DryRun

  prometheus:
    url: http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090

  aggregation: max  # max|min|average|weighted

  metrics:
    - name: http_rps
      promQL: |
        sum(rate(http_requests_total{app="laravel-web",route!~"/healthz"}[2m]))
      scaleUp:
        threshold: 250
        step: 3
      scaleDown:
        threshold: 80
        step: 1

    - name: http_latency_avg
      promQL: |
        rate(http_request_duration_seconds_sum{app="laravel-web"}[2m])
        /
        rate(http_request_duration_seconds_count{app="laravel-web"}[2m])
      scaleUp:
        threshold: 0.35    # 350ms
        step: 2
      scaleDown:
        threshold: 0.15    # 150ms
        step: 1

  behavior:
    stabilizationWindowSeconds: 120
    scaleUpCooldownSeconds: 60
    scaleDownCooldownSeconds: 180
    maxScaleUpStepPercent: 100   # at most 2x per decision
    maxScaleDownStepPercent: 50


Highlights:

targetRef supports arbitrary workloads (here Deployment).

Multiple metrics per autoscaler, each with its own scaleUp/scaleDown logic.

behavior config mimics and extends the tuning options of the built-in HPA.

Laravel Metrics & PromQL Signals

The demo focuses on a typical Laravel production stack:

Web: laravel-web (PHP-FPM + nginx or similar)

Queue workers: laravel-worker (Horizon or php artisan queue:work)

MySQL: tracked via mysqld_exporter

Redis: tracked via redis_exporter

Web Autoscaling

PromQL examples (used in laravel-web-autoscaler.yaml):

HTTP Requests per Second:

sum(rate(http_requests_total{app="laravel-web",route!~"/healthz"}[2m]))


Average Request Latency:

rate(http_request_duration_seconds_sum{app="laravel-web"}[2m])
/
rate(http_request_duration_seconds_count{app="laravel-web"}[2m])


(Optionally) Error Rate:

sum(rate(http_requests_total{app="laravel-web",status=~"5.."}[5m]))
/
sum(rate(http_requests_total{app="laravel-web"}[5m]))


These signals allow the controller to scale not only when CPU is high, but when latency and error rates suggest the app is under pressure.

Queue Worker Autoscaling

PromQL examples (used in laravel-queue-autoscaler.yaml):

Queue backlog (pending jobs):

sum(laravel_queue_jobs_pending{queue="emails"})


Age of the oldest job:

max(laravel_queue_oldest_job_age_seconds{queue="emails"})


When the backlog grows or jobs are sitting in the queue too long, the controller increases the worker replicas. Once the queue drains and latency drops, it scales back down.

MySQL & Redis (exporters)

The demo includes exporters:

mysqld_exporter for MySQL

redis_exporter for Redis

The same PrometheusAutoscaler CRD can be extended with PromQLs like:

# MySQL threads running
mysql_global_status_threads_running{instance="mysql:3306"}

# Redis memory ratio
redis_memory_used_bytes{instance="redis:6379"}
/
redis_memory_max_bytes{instance="redis:6379"}


These can act as safety gates: when the database or cache is saturated, the policy can choose to avoid aggressive scale-ups.

CI/CD: Jenkins + Helm + Argo CD

The repository is designed for a GitOps deployment model.

Jenkins

The Jenkinsfile:

Checks out the repo.

Runs unit tests: go test ./....

Builds the controller image from Dockerfile.

Tags the image with the short Git SHA.

Pushes the image to a container registry.

Updates deploy/helm/prometheus-autoscaler/values.yaml:

image:
  repository: registry.example.com/prometheus-autoscaler-controller
  tag: "<git-sha>"


Commits and pushes the updated values file back to Git.

No kubectl apply is run from Jenkins – instead it only updates Git state.

Argo CD

Two Argo CD Application objects (in deploy/argocd/):

prometheus-autoscaler-app.yaml

Points to deploy/helm/prometheus-autoscaler.

Installs/updates the controller in the cluster.

laravel-demo-app.yaml

Points to deploy/helm/laravel-demo.

Installs/updates the Laravel demo stack (web, worker, exporters, autoscalers).

Argo CD keeps the cluster in sync with the Git repository, automatically rolling out new controller images and application configs.

Repository Layout
prometheus-policy-autoscaler/
├── go.mod
├── Dockerfile
├── Jenkinsfile
├── cmd/
│   └── controller/
│       └── main.go                      # controller entrypoint
├── api/
│   └── v1alpha1/
│       ├── groupversion_info.go         # API version registration
│       └── prometheusautoscaler_types.go# CRD Go types
├── pkg/
│   ├── controller/
│   │   └── prometheusautoscaler_controller.go # reconciler logic
│   ├── metrics/
│   │   └── prometheus_client.go         # Prometheus HTTP client
│   ├── policy/
│   │   └── engine.go                    # scaling policy engine
│   └── history/
│       └── store.go                     # in-memory history store
├── config/
│   └── samples/                         # raw manifests for demo stack
│       ├── namespace.yaml
│       ├── laravel-web-deployment.yaml
│       ├── laravel-worker-deployment.yaml
│       ├── mysqld-exporter-deployment.yaml
│       ├── redis-exporter-deployment.yaml
│       ├── laravel-web-autoscaler.yaml
│       └── laravel-queue-autoscaler.yaml
└── deploy/
    ├── helm/
    │   ├── prometheus-autoscaler/       # Helm chart for controller
    │   └── laravel-demo/                # Helm chart for Laravel demo stack
    └── argocd/
        ├── prometheus-autoscaler-app.yaml
        └── laravel-demo-app.yaml

Getting Started

Requirements:

Go 1.22+

Docker

Kubernetes cluster (e.g. kind, k3d, Minikube, or managed)

Prometheus running in the cluster (e.g. kube-prometheus-stack)

(Optional) Argo CD & Jenkins for full CI/CD setup

1. Clone and build
git clone https://github.com/MreliotA/prometheus-policy-autoscaler.git
cd prometheus-policy-autoscaler

go mod tidy
go test ./...

2. Build the controller image
docker build -t your-registry/prometheus-autoscaler-controller:dev .
docker push your-registry/prometheus-autoscaler-controller:dev


Update deploy/helm/prometheus-autoscaler/values.yaml to use your image.

Deploying the Controller

With Helm:

helm upgrade --install prometheus-policy-autoscaler \
  deploy/helm/prometheus-autoscaler \
  --namespace monitoring \
  --create-namespace


This installs:

The Deployment for the controller.

RBAC and ServiceAccount.

Health and metrics endpoints.

Deploying the Laravel Demo Stack

You can use either raw YAML (config/samples) or the Helm chart (deploy/helm/laravel-demo).

Option A: Raw manifests
kubectl apply -f config/samples/namespace.yaml
kubectl apply -f config/samples/laravel-web-deployment.yaml
kubectl apply -f config/samples/laravel-worker-deployment.yaml
kubectl apply -f config/samples/mysqld-exporter-deployment.yaml
kubectl apply -f config/samples/redis-exporter-deployment.yaml
kubectl apply -f config/samples/laravel-web-autoscaler.yaml
kubectl apply -f config/samples/laravel-queue-autoscaler.yaml


Make sure your Laravel images expose metrics compatible with the PromQL expressions in the autoscaler specs (you can adjust the queries if needed).

Option B: Helm chart
helm upgrade --install laravel-demo \
  deploy/helm/laravel-demo \
  --namespace laravel-demo \
  --create-namespace \
  --set laravelWeb.image=your-registry/laravel-web:latest \
  --set laravelWorker.image=your-registry/laravel-worker:latest


This installs:

laravel-web Deployment + Service.

laravel-worker Deployment + Service.

mysqld-exporter + redis-exporter.

PrometheusAutoscaler CRs for both web and worker.

Extending to Other Workloads

Although the demo focuses on Laravel, the controller is intentionally generic:

The targetRef can point to any Deployment in any namespace.

Metrics are arbitrary PromQL expressions.

Policies are driven by configuration, not hard-coded logic.

To adapt this to another stack:

Instrument the application and dependencies with Prometheus metrics.

Create a new PrometheusAutoscaler CR:

Point spec.targetRef to your Deployment.

Fill spec.metrics with appropriate PromQL expressions.

Tune thresholds and behavior.

Apply the CR and observe the controller’s decisions via:

kubectl describe prometheusautoscaler <name>

Kubernetes Events.

Prometheus/Grafana dashboards.

Next Steps

Some ideas for future improvements:

Advanced aggregation strategies (e.g. “scale based on max of HTTP RPS and queue backlog”).

Per-metric gates (e.g. cap scale-ups when MySQL is near saturation).

Integration with KEDA or other autoscaling frameworks.

More extensive unit tests and integration tests for the policy engine.

Additional demos for non-Laravel workloads (e.g. Node.js, Go services).

If you’re interested in Prometheus-based autoscaling, Laravel observability, or GitOps workflows with Jenkins and Argo CD, this repository is meant to be a practical, end-to-end example you can experiment with, adapt, and extend.