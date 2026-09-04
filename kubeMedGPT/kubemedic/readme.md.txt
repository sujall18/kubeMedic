# KubeMedic

KubeMedic is a Go-based Kubernetes incident detection and remediation tool.

It monitors Kubernetes workloads, detects common application failures, collects diagnostic information, identifies potential causes, and can perform controlled remediation actions.

## Features

* Kubernetes pod monitoring
* Incident detection
* CrashLoopBackOff detection
* ImagePullBackOff detection
* Container restart tracking
* Exit code and termination reason analysis
* Previous container log collection
* Basic incident diagnosis
* Deployment rollback/remediation
* Remediation verification

## Architecture

```text
Kubernetes
    │
    ▼
KubeMedic
    │
    ├── Detect incident
    │
    ├── Collect evidence
    │      ├── Pod status
    │      ├── Restart count
    │      ├── Container state
    │      ├── Exit code
    │      └── Previous logs
    │
    ├── Diagnose
    │
    ├── Remediate
    │
    └── Verify recovery
```

## Tech Stack

* Go
* Kubernetes
* Kubernetes client-go
* Docker
* Minikube

## Example Incidents

### CrashLoopBackOff

KubeMedic can detect a repeatedly crashing container and collect information such as:

```text
Status: CrashLoopBackOff
Restarts: 17
Exit Code: 1
Previous Logs: CRASHING
```

It can then generate a diagnosis and recommend an appropriate action.

### ImagePullBackOff

KubeMedic can detect an invalid or unavailable container image and identify a potential deployment rollback.

Example:

```text
Image:
definitely-does-not-exist:kubemedic-test

Diagnosis:
Image pull failure

Remediation:
Rollback to previous known-good deployment revision
```

## Running Locally

Start Minikube:

```bash
minikube start
```

Run KubeMedic:

```bash
go run ./cmd
```

KubeMedic will connect to the Kubernetes cluster using the local kubeconfig and begin monitoring workloads.

## Testing Incidents

The project includes deliberately broken Kubernetes workloads for testing KubeMedic's detection and remediation capabilities.

For example:

```bash
kubectl apply -f test-app/
```

Then monitor the KubeMedic output:

```bash
go run ./cmd
```

You can inspect the Kubernetes workload with:

```bash
kubectl get pods
kubectl get deployments
kubectl describe pod <pod-name>
```

## Project Goal

KubeMedic is being developed as an engineering project to explore automated Kubernetes incident response.

The goal is not to replace Kubernetes' built-in self-healing capabilities. Instead, KubeMedic focuses on the layer above basic workload recovery:

```text
Detect
  ↓
Diagnose
  ↓
Choose safe remediation
  ↓
Apply remediation
  ↓
Verify recovery
```

Future development will focus on safer automated remediation, additional failure types, observability, testing, and production-oriented Kubernetes engineering practices.

## Status

🚧 **Work in progress**

The current implementation supports incident detection, diagnostics, and initial remediation workflows. More remediation strategies and production-hardening features are planned.
