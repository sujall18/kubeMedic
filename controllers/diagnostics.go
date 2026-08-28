package controllers

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type Diagnosis struct {
	Problem  string
	Reason   string
	Severity string
	Action   string
}

func DiagnoseContainer(
	status corev1.ContainerStatus,
	logs string,
) Diagnosis {

	// 1. OOMKilled
	if status.LastTerminationState.Terminated != nil {

		termination := status.LastTerminationState.Terminated

		if termination.Reason == "OOMKilled" {
			return Diagnosis{
				Problem:  "Container ran out of memory",
				Reason:   "Container was terminated by the kernel",
				Severity: "HIGH",
				Action:   "Increase memory limit or investigate memory usage",
			}
		}
	}

	// 2. Database connection failure
	if strings.Contains(
		strings.ToLower(logs),
		"database_connection_failed",
	) {
		return Diagnosis{
			Problem:  "Application dependency failure",
			Reason:   "Application reported a database connection failure",
			Severity: "HIGH",
			Action:   "Check database availability, credentials and network connectivity",
		}
	}

	// 3. Connection refused
	if strings.Contains(
		strings.ToLower(logs),
		"connection refused",
	) {
		return Diagnosis{
			Problem:  "Connection refused",
			Reason:   "Application could not establish a network connection",
			Severity: "HIGH",
			Action:   "Check the target service, port and network connectivity",
		}
	}

	// 4. Permission problem
	if strings.Contains(
		strings.ToLower(logs),
		"permission denied",
	) {
		return Diagnosis{
			Problem:  "Permission failure",
			Reason:   "Application does not have permission to access a resource",
			Severity: "HIGH",
			Action:   "Check filesystem permissions, security context and RBAC",
		}
	}

	// 5. Exit code 1
	if status.LastTerminationState.Terminated != nil {

		exitCode := status.LastTerminationState.Terminated.ExitCode

		if exitCode == 1 {
			return Diagnosis{
				Problem:  "Application repeatedly crashing",
				Reason:   "Container exited with code 1",
				Severity: "HIGH",
				Action:   "Inspect application logs and startup configuration",
			}
		}
	}

	// 6. Unknown failure
	return Diagnosis{
		Problem:  "Unknown container failure",
		Reason:   "KubeMedic could not determine the root cause",
		Severity: "MEDIUM",
		Action:   "Inspect container logs and Kubernetes events",
	}
}
