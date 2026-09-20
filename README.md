# KubeMedic

**Kubernetes Incident Detection & Automated Remediation Engine**

KubeMedic is a Go-based Kubernetes incident-response system that monitors workloads, diagnoses common container failures, applies controlled remediation policies, and verifies workload recovery.

The project was built to explore how automated incident response can operate above Kubernetes' built-in self-healing mechanisms.

## What problem does it solve?

Kubernetes can automatically restart failed containers and maintain the desired number of replicas, but restarting a workload does not necessarily resolve the underlying cause.

For example:

```text
Application crashes
       ↓
Kubernetes restarts container
       ↓
Application crashes again
       ↓
CrashLoopBackOff
       ↓
Kubernetes keeps attempting recovery
```

KubeMedic adds a diagnosis and remediation layer:

```text
Failure
   ↓
Detection
   ↓
Diagnosis
   ↓
Remediation Policy
   ↓
Automated Remediation
   ↓
Verification
   ↓
Resolved / Failed
```

## Features

### Incident Detection

KubeMedic monitors Kubernetes Pods and detects conditions including:

* `CrashLoopBackOff`
* `OOMKilled`
* `ImagePullBackOff`
* `ErrImagePull`

Future work

* database connection failures
* connection refused errors
* permission failures
* container exit failures

KubeMedic performs an initial scan of existing Pods and then watches for subsequent Kubernetes events.

### Diagnosis

Incidents are analysed using Kubernetes workload state and container evidence, including:

* container termination reasons
* exit codes
* previous container logs
* Pod status
* Deployment and ReplicaSet relationships

Diagnoses are represented using structured information such as:

```text
Problem
Reason
Severity
Recommended action
```

### Automated OOM Remediation

For memory-related failures, KubeMedic can automatically increase the Deployment's memory limit according to a bounded remediation policy.

Example:

```text
64Mi
  ↓
128Mi
  ↓
256Mi
```

The remediation is performed through the Kubernetes API rather than by modifying local YAML files.

Automatic remediation is bounded to prevent uncontrolled resource escalation.

### Deployment Rollback

For image-related failures such as `ImagePullBackOff`, KubeMedic can:

1. Identify the affected Deployment.
2. Inspect Deployment revision history.
3. Locate a previous revision.
4. Restore the previous Pod template.
5. Update the Deployment through the Kubernetes API.
6. Verify the resulting workload state.

Example:

```text
Working Deployment
       ↓
Broken container image
       ↓
ImagePullBackOff
       ↓
KubeMedic diagnosis
       ↓
Previous Deployment revision
       ↓
Rollback
       ↓
Workload recovery
```

### Incident State Management

KubeMedic tracks incident handling state to prevent repeated remediation attempts for the same incident and to maintain bounded remediation behaviour.

## Architecture

```text
                 Kubernetes Cluster
                        │
                        ▼
                 ┌─────────────┐
                 │   Watcher   │
                 └──────┬──────┘
                        │
                        ▼
                 ┌─────────────┐
                 │  Detection  │
                 └──────┬──────┘
                        │
                        ▼
                 ┌─────────────┐
                 │  Diagnosis  │
                 └──────┬──────┘
                        │
                        ▼
                 ┌─────────────┐
                 │   Policy    │
                 │    Engine   │
                 └──────┬──────┘
                        │
                        ▼
                 ┌─────────────┐
                 │ Remediation │
                 └──────┬──────┘
                        │
                        ▼
                 ┌─────────────┐
                 │ Verification│
                 └──────┬──────┘
                        │
                  ┌─────┴─────┐
                  ▼           ▼
              RESOLVED      FAILED
```

## Technology Stack

* **Go**
* **Kubernetes**
* **Kubernetes client-go**
* **Docker**
* **Minikube**
* **Linux / WSL**
* Kubernetes Deployments
* ReplicaSets
* Kubernetes API
* Container logs and lifecycle state

## Project Structure

```text
kubeMedic/
├── cmd/
│   └── main.go
├── controllers/
│   ├── diagnostics.go
│   ├── incidents.go
│   ├── pod_analyzer.go
│   ├── remediation.go
│   └── watcher.go
├── test-app/
│   ├── oom-broken.yaml
│   ├── oom-success.yaml
│   ├── imagePullErr.yaml
│   ├── remediation-demo.yaml
│   └── remediation-broken.yaml
├── go.mod
└── README.md
```

## Running KubeMedic

### Prerequisites

Install:

* Go
* Docker
* kubectl
* Minikube

Start a local Kubernetes cluster:

```bash
minikube start --driver=docker
```

Clone the repository and enter the project:

```bash
git clone https://github.com/sujall18/kubeMedic.git
cd kubeMedic
```

Install dependencies:

```bash
go mod tidy
```

Build and test:

```bash
go test ./...
go build ./...
```

Start KubeMedic:

```bash
go run ./cmd
```

## Testing OOM Remediation

Deploy the intentionally failing workload:

```bash
kubectl apply -f test-app/oom-broken.yaml
```

Start KubeMedic:

```bash
go run ./cmd
```

KubeMedic detects the OOM failure and attempts bounded memory escalation.

Example:

```text
🚨 INCIDENT DETECTED

Diagnosis:
  Problem: Container exceeded memory limit
  Reason: OOM_KILLED
  Severity: CRITICAL
  Recommended action: Increase memory limit

⚕️ OOM remediation attempt 1/2
Memory: 64Mi → 128Mi
```

The live Kubernetes Deployment is updated through the Kubernetes API.

Verify the resulting resource:

```bash
kubectl get deployment memory-hog \
  -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}{"\n"}'
```

## Testing Deployment Rollback

Deploy a healthy version:

```bash
kubectl apply -f test-app/remediation-demo.yaml
```

Introduce a broken image:

```bash
kubectl set image deployment/broken-app \
  broken-app=definitely-does-not-exist:kubemedic-test
```

KubeMedic detects the resulting image-pull failure and can use the previous Deployment revision for rollback.

## Engineering Principles

KubeMedic is designed around several principles:

### Diagnose before acting

A Kubernetes restart is not necessarily a remediation. KubeMedic first attempts to determine the failure cause.

### Bounded automation

Automatic remediation must have explicit limits rather than continuously modifying resources.

### Verify after remediation

A successful Kubernetes API update does not automatically mean the application recovered. Remediation should be followed by workload verification.

### Kubernetes API over shell commands

The controller interacts with Kubernetes through `client-go` rather than depending on shelling out to `kubectl` for remediation.

### Idempotent incident handling

Incident state is tracked to avoid repeatedly acting on the same failure event.

## What KubeMedic Demonstrates

This project demonstrates practical experience with:

* Kubernetes workload lifecycle
* Kubernetes API programming
* Go systems development
* container failure analysis
* incident-response workflows
* automated remediation
* Deployment and ReplicaSet management
* rollout and rollback concepts
* resource management
* failure handling
* bounded automation
* verification-driven recovery
* debugging distributed/containerised workloads

## Project Status

KubeMedic is a **working engineering prototype** demonstrating Kubernetes incident detection, diagnosis, controlled remediation and recovery workflows.

Working on Observability 

It is not intended to replace Kubernetes controllers, operators or production incident-management platforms. The project focuses on demonstrating the engineering principles behind automated Kubernetes incident response.
