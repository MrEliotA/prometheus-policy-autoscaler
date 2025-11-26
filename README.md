Prometheus Policy Autoscaler

Prometheus-driven Kubernetes autoscaler for Laravel-based workloads (web and queues).
Implements policy-as-code, PromQL-based scaling logic, and integrates into a GitOps-friendly CI/CD pipeline using Jenkins, Helm, and Argo CD.

A custom controller that understands application-level and database-level signals—not just CPU—and scales based on Laravel HTTP traffic, queue backlog, MySQL pressure, and Redis saturation.

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

Kubernetes Horizontal Pod Autoscalers are effective for CPU or memory-driven workloads, but real applications often require richer context. Laravel monoliths typically rely on:

HTTP request throughput

Request latency and error rates

Queue backlog and job age

MySQL concurrency and connection saturation

Redis memory and performance indicators

This project provides a Kubernetes controller written in Go that:

Defines a CRD (PrometheusAutoscaler) expressing autoscaling logic as PromQL policies

Targets Laravel stacks (web and workers) as an example, but can support any workload

Uses Prometheus as the sole metrics source

Is deployed via Helm and managed through Argo CD (GitOps)

Is built and released using a Jenkins pipeline

Key Features
Custom Resource Definition

PrometheusAutoscaler (API group: autoscaler.laravel.app/v1alpha1).

Policy-as-code

Multiple metrics per autoscaler

Per-metric scaleUp and scaleDown thresholds and step sizes

Aggregation strategies: max, min, average, weighted

Stabilization windows, cooldown behavior, and rate limits

Laravel-aware scaling signals

HTTP RPS, average and percentile latency, error rate

Queue backlog and oldest job age (Horizon-compatible metrics)

MySQL thread and connection metrics via mysqld_exporter

Redis memory usage via redis_exporter

Dry-run mode

Simulate decisions without patching Deployments.

Implementation details

Built with controller-runtime

Structured logging, readiness and liveness probes

Emits Kubernetes Events describing scaling decisions

GitOps-friendly deployment

Controller packaged as a Helm chart

Argo CD tracks Git and applies changes automatically

Jenkins updates image tags in Helm values

High-Level Architecture
Control Loop

PrometheusAutoscaler objects are created in the cluster.

The controller watches these CRs.

For each CR:

Fetch the referenced Deployment

Execute PromQL queries against Prometheus

Feed metric samples into the policy engine

The policy engine computes desiredReplicas based on:

Per-metric thresholds

Aggregation strategy

Stabilization windows and cooldowns

Scale-up/scale-down rate limits

If not in DryRun mode and the replica count changes, the controller patches the Deployment.

The CR status is updated with:

currentReplicas

desiredReplicas

lastScaleTime

metric samples used in the decision

CI/CD Flow

Developer pushes to GitHub

Jenkins pipeline:

Runs go test ./...

Builds Docker image

Tags with commit SHA

Pushes to registry

Updates values.yaml via yq

Commits and pushes changes back to Git

Argo CD detects chart changes

Argo CD syncs the updated controller into Kubernetes

Custom Resource: PrometheusAutoscaler

Example (simplified):

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

HTTP Requests per Second:

sum(rate(http_requests_total{app="laravel-web",route!~"/healthz"}[2m]))


Average Request Latency:

rate(http_request_duration_seconds_sum{app="laravel-web"}[2m])
/
rate(http_request_duration_seconds_count{app="laravel-web"}[2m])


Error Rate (optional):

sum(rate(http_requests_total{app="laravel-web",status=~"5.."}[5m]))
/
sum(rate(http_requests_total{app="laravel-web"}[5m]))

Queue Autoscaling

Queue Backlog:

sum(laravel_queue_jobs_pending{queue="emails"})


Oldest Job Age:

max(laravel_queue_oldest_job_age_seconds{queue="emails"})

MySQL and Redis Signals

MySQL Running Threads:

mysql_global_status_threads_running{instance="mysql:3306"}


Redis Memory Ratio:

redis_memory_used_bytes / redis_memory_max_bytes

CI/CD: Jenkins, Helm, and Argo CD
Jenkins Pipeline

Runs tests

Builds and pushes Docker image

Updates helm chart values (image.tag)

Pushes changes back to Git

Does not apply manifests directly

Argo CD

Two Applications:

prometheus-autoscaler-app.yaml
Deploys the controller Helm chart.

laravel-demo-app.yaml
Deploys a demo Laravel stack including:

Web deployment

Queue workers

MySQL and Redis exporters

Autoscaler CRs

Argo CD keeps the cluster synchronized with Git.

Repository Layout
prometheus-policy-autoscaler/
├── go.mod
├── Dockerfile
├── Jenkinsfile
├── cmd/
│   └── controller/
│       └── main.go
├── api/
│   └── v1alpha1/
│       ├── groupversion_info.go
│       └── prometheusautoscaler_types.go
├── pkg/
│   ├── controller/
│   │   └── prometheusautoscaler_controller.go
│   ├── metrics/
│   │   └── prometheus_client.go
│   ├── policy/
│   │   └── engine.go
│   └── history/
│       └── store.go
├── config/
│   └── samples/
│       ├── namespace.yaml
│       ├── laravel-web-deployment.yaml
│       ├── laravel-worker-deployment.yaml
│       ├── mysqld-exporter-deployment.yaml
│       ├── redis-exporter-deployment.yaml
│       ├── laravel-web-autoscaler.yaml
│       └── laravel-queue-autoscaler.yaml
└── deploy/
    ├── helm/
    │   ├── prometheus-autoscaler/
    │   └── laravel-demo/
    └── argocd/
        ├── prometheus-autoscaler-app.yaml
        └── laravel-demo-app.yaml

Getting Started
Requirements

Go 1.22+

Docker

Kubernetes cluster (kind, k3d, Minikube, managed)

Prometheus installed (e.g., kube-prometheus-stack)

Optional: Argo CD and Jenkins

Build
git clone https://github.com/MreliotA/prometheus-policy-autoscaler.git
cd prometheus-policy-autoscaler

go mod tidy
go test ./...

Build the image
docker build -t your-registry/prometheus-autoscaler-controller:dev .
docker push your-registry/prometheus-autoscaler-controller:dev


Update Helm values accordingly.

Deploying the Controller
helm upgrade --install prometheus-policy-autoscaler \
  deploy/helm/prometheus-autoscaler \
  --namespace monitoring \
  --create-namespace


This installs:

Controller Deployment

RBAC resources

ServiceAccount

Health and metrics endpoints

Deploying the Laravel Demo Stack
Option A: Raw Manifests
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

The controller is generic and can autoscale any Kubernetes Deployment.

To adapt it:

Expose Prometheus metrics from your application

Create a new PrometheusAutoscaler CR pointing to your Deployment

Define PromQL queries and thresholds

Tune behavior settings

Inspect decisions with:

kubectl describe prometheusautoscaler <name>

Next Steps

Possible future improvements:

More advanced aggregation logic

Per-metric gating (e.g., limit scale-up when MySQL is overloaded)

Optional integration with KEDA

Additional test coverage for policy engine

Demo stacks for non-Laravel workloads