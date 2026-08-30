package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

func AnalyzePod(
	ctx context.Context,
	clientset kubernetes.Interface,
	pod *corev1.Pod,
	tracker *IncidentTracker,
) {

	for _, containerStatus := range pod.Status.ContainerStatuses {

		if containerStatus.State.Waiting == nil ||
			containerStatus.State.Waiting.Reason != "CrashLoopBackOff" {

			key := fmt.Sprintf(
				"%s/%s/%s",
				pod.Namespace,
				pod.Name,
				containerStatus.Name,
			)

			tracker.Resolve(key)

			continue
		}

		reason := containerStatus.State.Waiting.Reason

		if reason != "CrashLoopBackOff" {
			continue
		}

		fmt.Println()
		fmt.Println("🚨 INCIDENT DETECTED")
		fmt.Println("----------------------------------------")

		fmt.Printf(
			"Pod: %s/%s\n",
			pod.Namespace,
			pod.Name,
		)

		fmt.Printf(
			"Container: %s\n",
			containerStatus.Name,
		)

		fmt.Printf(
			"Status: %s\n",
			reason,
		)

		fmt.Printf(
			"Restarts: %d\n",
			containerStatus.RestartCount,
		)

		// Get previous container logs
		logs, err := clientset.CoreV1().
			Pods(pod.Namespace).
			GetLogs(
				pod.Name,
				&corev1.PodLogOptions{
					Container: containerStatus.Name,
					Previous:  true,
				},
			).
			Do(ctx).
			Raw()

		logText := ""

		if err != nil {
			fmt.Printf("Could not retrieve logs: %v\n", err)
		} else {
			logText = string(logs)

			fmt.Printf(
				"Previous logs:\n%s\n",
				logText,
			)
		}

		diagnosis := DiagnoseContainer(
			containerStatus,
			logText,
		)

		fmt.Println()
		fmt.Println("Diagnosis:")
		fmt.Printf("  Problem: %s\n", diagnosis.Problem)
		fmt.Printf("  Reason: %s\n", diagnosis.Reason)
		fmt.Printf("  Severity: %s\n", diagnosis.Severity)
		fmt.Printf("  Recommended action: %s\n", diagnosis.Action)

		fmt.Println("----------------------------------------")

		if ShouldRemediate(diagnosis) {

			fmt.Println()
			fmt.Println("🤖 KubeMedic will attempt automatic remediation")

			err := RestartPod(
				ctx,
				clientset,
				pod,
			)

			if err != nil {
				fmt.Printf(
					"❌ Remediation failed: %v\n",
					err,
				)
			}
		}
	}
}
