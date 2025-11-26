Prometheus Policy Autoscaler

A Prometheus-driven, policy-as-code Kubernetes autoscaler designed for Laravel-based workloads (web and queues), providing scaling based on real application metrics such as HTTP throughput, latency, queue backlog, MySQL pressure, and Redis memory saturation.

This project implements a custom Kubernetes controller that evaluates PromQL-based signals, applies configurable scaling policies, and manages replica adjustments for any Deployment. It integrates fully with GitOps flows using Helm and Argo CD and provides an example CI/CD pipeline via Jenkins.

Navigation

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

Traditional Kubernetes HPAs use CPU, memory, or a single custom metric. Real-world Laravel applications need more context:

HTTP request rate and latency

Queue backlog and job age

Database and cache pressure

Error rates and saturation signals

This project introduces:

A Kubernetes controller written in Go

A CRD (PrometheusAutoscaler) expressing autoscaling logic as PromQL policies

Generic enough to support any Deployment (not only Laravel)

GitOps-friendly deployment via Helm and Argo CD

CI/CD pipeline powered by Jenkins

Key Features
PrometheusAutoscaler CRD (autoscaler.laravel.app/v1alpha1)
Policy-as-code

Multiple metrics per autoscaler

Separate scale-up and scale-down thresholds

Scaling steps and rate controls

Stabilization windows

Aggregation strategies: max, min, average, weighted

Laravel-specific metrics

HTTP throughput

Request latency and error rate

Queue backlog & oldest job age

MySQL connection/thread load

Redis memory usage

DryRun Mode

Simulates decisions without updating the Deployment.

Controller implementation

Based on controller-runtime

Health/readiness probes

Structured logging

Kubernetes Events for visibility

GitOps Support

Helm chart for the controller

Argo CD watches Git and syncs automatically

Jenkins updates image tags in the chart

High-Level Architecture
Control Flow
PrometheusAutoscaler CR → PromQL evaluation → Policy Engine → desiredReplicas → Deployment patch → Status update


Watch PrometheusAutoscaler resources

Fetch referenced Deployment

Query Prometheus using PromQL expressions

Evaluate metrics inside policy engine

Compute the desired number of replicas

Patch Deployment (unless DryRun)

Update CR status with metrics snapshot

CI/CD Pipeline
GitHub → Jenkins → Docker Registry → Git (updated Helm values) → Argo CD → Kubernetes

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

HTTP Requests Per Second

sum(rate(http_requests_total{app="laravel-web",route!~"/healthz"}[2m]))


Average Request Latency

rate(http_request_duration_seconds_sum{app="laravel-web"}[2m])
/
rate(http_request_duration_seconds_count{app="laravel-web"}[2m])


Error Rate

sum(rate(http_requests_total{app="laravel-web",status=~"5.."}[5m]))
/
sum(rate(http_requests_total{app="laravel-web"}[5m]))

Queue Autoscaling

Queue backlog

sum(laravel_queue_jobs_pending{queue="emails"})


Oldest job age

max(laravel_queue_oldest_job_age_seconds{queue="emails"})

MySQL & Redis Exporters

MySQL threads running

mysql_global_status_threads_running


Redis memory ratio

redis_memory_used_bytes / redis_memory_max_bytes

CI/CD: Jenkins, Helm, and Argo CD
Jenkins Pipeline

go test ./...

Build Docker image

Tag with commit SHA

Push to registry

Modify Helm values.yaml with new tag

Commit and push updates

Argo CD

Two application definitions:

prometheus-autoscaler-app.yaml — deploys controller Helm chart

laravel-demo-app.yaml — deploys Laravel demo stack

Argo CD monitors Git, detects changes, and syncs automatically.

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

Prometheus installed (kube-prometheus-stack recommended)

Optional: Argo CD and Jenkins

Build
git clone https://github.com/MreliotA/prometheus-policy-autoscaler.git
cd prometheus-policy-autoscaler

go mod tidy
go test ./...

Build Image
docker build -t your-registry/prometheus-autoscaler-controller:dev .
docker push your-registry/prometheus-autoscaler-controller:dev


Update Helm chart to use your image tag.

Deploying the Controller
helm upgrade --install prometheus-policy-autoscaler \
  deploy/helm/prometheus-autoscaler \
  --namespace monitoring \
  --create-namespace

Deploying the Laravel Demo Stack
Raw YAML
kubectl apply -f config/samples/namespace.yaml
kubectl apply -f config/samples/laravel-web-deployment.yaml
kubectl apply -f config/samples/laravel-worker-deployment.yaml
kubectl apply -f config/samples/mysqld-exporter-deployment.yaml
kubectl apply -f config/samples/redis-exporter-deployment.yaml
kubectl apply -f config/samples/laravel-web-autoscaler.yaml
kubectl apply -f config/samples/laravel-queue-autoscaler.yaml

Helm Chart
helm upgrade --install laravel-demo \
  deploy/helm/laravel-demo \
  --namespace laravel-demo \
  --create-namespace \
  --set laravelWeb.image=your-registry/laravel-web:latest \
  --set laravelWorker.image=your-registry/laravel-worker:latest

Extending to Other Workloads

To autoscale another Deployment:

Expose Prometheus metrics

Create a new PrometheusAutoscaler referencing it

Write PromQL expressions for metrics

Tune thresholds, aggregation, and behavior

Observe controller behavior:

kubectl describe prometheusautoscaler <name>

Next Steps

Additional aggregation strategies

Metric gating logic (e.g., block scale-up when MySQL is saturated)

Optional KEDA integration

More test coverage

Demo stacks for Node.js, Go, etc.