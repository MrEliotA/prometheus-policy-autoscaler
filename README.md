Prometheus Policy Autoscaler

Prometheus-driven Kubernetes autoscaler for Laravel-based workloads (web and queues), featuring policy-as-code, PromQL integration, and a complete GitOps-friendly CI/CD pipeline (Jenkins, Helm, Argo CD).

This controller scales applications based on real signals such as HTTP traffic, queue backlog, MySQL load, and Redis pressure, instead of relying solely on CPU and memory usage.

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

Kubernetes Horizontal Pod Autoscalers typically respond to CPU, memory, or a single custom metric.
However, real Laravel production systems require broader context:

HTTP request throughput

Latency (p95, average)

Queue backlog and job age

MySQL thread/connection pressure

Redis memory saturation

This project provides:

A Kubernetes controller written in Go

A CRD (PrometheusAutoscaler) describing autoscaling policies as code

Prometheus as the single metrics source

GitOps deployment via Helm and Argo CD

A Jenkins-driven CI pipeline for builds and releases

Key Features
Custom Resource Definition: PrometheusAutoscaler

API group: autoscaler.laravel.app/v1alpha1.

Policy-as-code

Multiple metrics per policy

Separate scaleUp and scaleDown rules

Aggregation strategies: max, min, average, weighted

Stabilization windows and cooldown mechanisms

Rate limiting for scaling adjustments

Laravel-aware autoscaling signals

HTTP RPS

Average and percentile latency

Error rate

Queue backlog, waiting job age

MySQL threads, connections

Redis memory usage

Dry-run mode

Simulates scaling decisions without applying changes.

Implementation details

Built on controller-runtime

Structured logging

Health/readiness probes

Kubernetes Events for visibility

GitOps-friendly

Controller packaged as a Helm chart

Argo CD synchronizes configuration from Git

Jenkins updates image tags automatically

High-Level Architecture
Control Loop Overview

Watches for PrometheusAutoscaler resources.

Retrieves referenced Deployments.

Executes PromQL queries against Prometheus.

Evaluates results using policy engine.

Computes desiredReplicas.

Applies changes (unless in DryRun mode).

Updates .status with:

currentReplicas

desiredReplicas

lastScaleTime

metrics snapshot

CI/CD Flow

Developer pushes to Git.

Jenkins:

Runs unit tests

Builds Docker image

Tags with commit SHA

Pushes to registry

Updates Helm values.yaml with new image tag

Argo CD detects changes and deploys updates automatically.

Custom Resource: PrometheusAutoscaler

Example:

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

HTTP Requests Per Second:

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

Backlog:

sum(laravel_queue_jobs_pending{queue="emails"})


Oldest Job Age:

max(laravel_queue_oldest_job_age_seconds{queue="emails"})

MySQL Example
mysql_global_status_threads_running

Redis Example
redis_memory_used_bytes / redis_memory_max_bytes

CI/CD: Jenkins, Helm, and Argo CD
Jenkins Pipeline

Runs Go unit tests

Builds and pushes controller image

Updates Helm chart values with new image tag

Commits changes to Git

Argo CD Applications

prometheus-autoscaler-app.yaml: Deploys controller

laravel-demo-app.yaml: Deploys demo Laravel stack

Argo CD continuously syncs cluster state with Git.

Repository Layout
prometheus-policy-autoscaler/
├── go.mod
├── Dockerfile
├── Jenkinsfile
├── cmd/controller/main.go
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

Prometheus installation

Optional: Argo CD and Jenkins

Build
git clone https://github.com/MreliotA/prometheus-policy-autoscaler.git
cd prometheus-policy-autoscaler

go mod tidy
go test ./...

Build Image
docker build -t your-registry/prometheus-autoscaler-controller:dev .
docker push your-registry/prometheus-autoscaler-controller:dev


Update Helm values to reflect image tag.

Deploying the Controller
helm upgrade --install prometheus-policy-autoscaler \
  deploy/helm/prometheus-autoscaler \
  --namespace monitoring \
  --create-namespace

Deploying the Laravel Demo Stack
Raw Manifests
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

Expose Prometheus metrics in your application

Create a PrometheusAutoscaler referencing your Deployment

Define PromQL expressions

Tune thresholds and behavior

Inspect controller decisions:

kubectl describe prometheusautoscaler <name>

Next Steps

Additional aggregation strategies

Per-metric gating and safety conditions

Potential integration with KEDA

Expanded test coverage

Examples for non-Laravel workloads