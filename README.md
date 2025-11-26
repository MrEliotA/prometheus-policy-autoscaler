Prometheus Policy Autoscaler

Prometheus-driven Kubernetes autoscaler for Laravel-based workloads (web and queues).
Implements policy-as-code, PromQL-based scaling, and integrates with GitOps workflows using Jenkins, Helm, and Argo CD.

A custom controller that understands application-level and infrastructure-level signals—Laravel HTTP traffic, queue pressure, MySQL threads, Redis usage—and scales accordingly.

Table of Contents

Motivation

Key Features

High-Level Architecture

Custom Resource: PrometheusAutoscaler

Laravel Metrics and PromQL Signals

CI/CD: Jenkins, Helm, and Argo CD

Repository Layout

Getting Started

Deploying the Controller

Deploying the Laravel Demo Stack

Extending to Other Workloads

Next Steps

Motivation

Kubernetes HPAs usually rely on CPU/memory or a single metric. Real Laravel systems require more context:

HTTP request throughput

Request latency

Queue backlog and job age

MySQL thread/connection pressure

Redis memory usage

This controller:

Defines a CRD (PrometheusAutoscaler) where scaling logic is expressed via PromQL

Targets Laravel workloads as an example but supports any Deployment

Uses Prometheus as the metrics source

Is deployed via Helm and managed through Argo CD

Is built through Jenkins CI

Key Features
Custom Resource Definition

PrometheusAutoscaler (autoscaler.laravel.app/v1alpha1)

Policy-as-code

Multiple metrics per autoscaler

Separate scaleUp and scaleDown rules per metric

Aggregation strategies: max, min, average, weighted

Stabilization, cooldown, and rate limit features

Laravel-focused autoscaling signals

HTTP RPS, latency, error rate

Queue backlog and oldest job age

MySQL exporter metrics

Redis exporter metrics

Dry-run mode

Test decisions without making changes.

Implementation features

Based on controller-runtime

Readiness/liveness probes

Kubernetes Events for decision logs

GitOps friendly

Helm deployment

Argo CD synchronization

Jenkins image build + values update

High-Level Architecture
Control Loop

Watches PrometheusAutoscaler CRs

Fetches corresponding Deployments

Executes PromQL queries

Feeds values into the policy engine

Produces desiredReplicas

Applies changes (unless DryRun)

Updates CR .status

CI/CD Flow

Developer pushes to GitHub

Jenkins:

Runs tests

Builds Docker image

Tags image with commit SHA

Pushes the image

Updates Helm values.yaml

Jenkins pushes changes to Git

Argo CD detects changes and syncs to cluster

Custom Resource: PrometheusAutoscaler
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
  mode: Apply

  prometheus:
    url: http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090

  aggregation: max

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
        threshold: 0.35
        step: 2
      scaleDown:
        threshold: 0.15
        step: 1

  behavior:
    stabilizationWindowSeconds: 120
    scaleUpCooldownSeconds: 60
    scaleDownCooldownSeconds: 180
    maxScaleUpStepPercent: 100
    maxScaleDownStepPercent: 50

Laravel Metrics and PromQL Signals
Web Autoscaling

HTTP RPS:

sum(rate(http_requests_total{app="laravel-web",route!~"/healthz"}[2m]))


Average Latency:

rate(http_request_duration_seconds_sum{app="laravel-web"}[2m])
/
rate(http_request_duration_seconds_count{app="laravel-web"}[2m])


Error Rate:

sum(rate(http_requests_total{app="laravel-web",status=~"5.."}[5m]))
/
sum(rate(http_requests_total{app="laravel-web"}[5m]))

Queue Autoscaling

Queue Backlog:

sum(laravel_queue_jobs_pending{queue="emails"})


Oldest Job Age:

max(laravel_queue_oldest_job_age_seconds{queue="emails"})

MySQL Signals
mysql_global_status_threads_running

Redis Signals
redis_memory_used_bytes / redis_memory_max_bytes

CI/CD: Jenkins, Helm, and Argo CD
Jenkins Pipeline

Runs Go tests

Builds and pushes Docker image

Modifies Helm values (image.tag)

Pushes back to Git

Does not apply to cluster directly

Argo CD Applications

prometheus-autoscaler-app.yaml (controller)

laravel-demo-app.yaml (demo stack)

Argo CD syncs the cluster with Git state.

Repository Layout
prometheus-policy-autoscaler/
├── go.mod
├── Dockerfile
├── Jenkinsfile
├── cmd/
│   └── controller/main.go
├── api/v1alpha1/
│   ├── groupversion_info.go
│   └── prometheusautoscaler_types.go
├── pkg/
│   ├── controller/prometheusautoscaler_controller.go
│   ├── metrics/prometheus_client.go
│   ├── policy/engine.go
│   └── history/store.go
├── config/samples/
│   ├── namespace.yaml
│   ├── laravel-web-deployment.yaml
│   ├── laravel-worker-deployment.yaml
│   ├── mysqld-exporter-deployment.yaml
│   ├── redis-exporter-deployment.yaml
│   ├── laravel-web-autoscaler.yaml
│   └── laravel-queue-autoscaler.yaml
└── deploy/
    ├── helm/
    │   ├── prometheus-autoscaler
    │   └── laravel-demo
    └── argocd/
        ├── prometheus-autoscaler-app.yaml
        └── laravel-demo-app.yaml

Getting Started
Requirements

Go 1.22+

Docker

Kubernetes cluster

Prometheus installation (kube-prometheus-stack recommended)

Optional: Argo CD, Jenkins

Build
git clone https://github.com/MreliotA/prometheus-policy-autoscaler.git
cd prometheus-policy-autoscaler

go mod tidy
go test ./...

Build and push image
docker build -t your-registry/prometheus-autoscaler-controller:dev .
docker push your-registry/prometheus-autoscaler-controller:dev


Update helm values to use the new tag.

Deploying the Controller
helm upgrade --install prometheus-policy-autoscaler \
  deploy/helm/prometheus-autoscaler \
  --namespace monitoring \
  --create-namespace

Deploying the Laravel Demo Stack
Option A: Raw YAML
kubectl apply -f config/samples/namespace.yaml
kubectl apply -f config/samples/laravel-web-deployment.yaml
kubectl apply -f config/samples/laravel-worker-deployment.yaml
kubectl apply -f config/samples/mysqld-exporter-deployment.yaml
kubectl apply -f config/samples/redis-exporter-deployment.yaml
kubectl apply -f config/samples/laravel-web-autoscaler.yaml
kubectl apply -f config/samples/laravel-queue-autoscaler.yaml

Option B: Helm Chart
helm upgrade --install laravel-demo \
  deploy/helm/laravel-demo \
  --namespace laravel-demo \
  --create-namespace \
  --set laravelWeb.image=your-registry/laravel-web:latest \
  --set laravelWorker.image=your-registry/laravel-worker:latest

Extending to Other Workloads

Instrument your service with Prometheus metrics

Create a new PrometheusAutoscaler

Define PromQL expressions

Configure thresholds and behavior

Inspect decisions:

kubectl describe prometheusautoscaler <name>

Next Steps

More aggregation strategies

Per-metric gating logic

Possible integration with KEDA

Additional test suites

Demo stacks for other ecosystems