package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/sujall18/kubeMedic/controllers"
)

func main() {
	fmt.Println("KubeMedic starting...")

	config, err := getKubeConfig()
	if err != nil {
		panic(err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	pods, err := clientset.CoreV1().Pods("default").List(
		context.Background(),
		metav1.ListOptions{},
	)
	if err != nil {
		panic(err)
	}

	for _, pod := range pods.Items {
		fmt.Printf(
			"Pod: %s | Namespace: %s | Phase: %s\n",
			pod.Name,
			pod.Namespace,
			pod.Status.Phase,
		)

		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.State.Waiting == nil {
				continue
			}

			reason := containerStatus.State.Waiting.Reason

			if reason != "CrashLoopBackOff" {
				continue
			}

			fmt.Printf(
				"\n🚨 INCIDENT DETECTED\n"+
					"Pod: %s/%s\n"+
					"Container: %s\n"+
					"Status: %s\n"+
					"Restarts: %d\n",
				pod.Namespace,
				pod.Name,
				containerStatus.Name,
				reason,
				containerStatus.RestartCount,
			)

			lastState := containerStatus.LastTerminationState

			if lastState.Terminated != nil {
				fmt.Printf(
					"Last termination reason: %s\n",
					lastState.Terminated.Reason,
				)

				fmt.Printf(
					"Exit code: %d\n",
					lastState.Terminated.ExitCode,
				)
			}

			logs, err := clientset.CoreV1().Pods(pod.Namespace).GetLogs(
				pod.Name,
				&corev1.PodLogOptions{
					Container: containerStatus.Name,
					Previous:  true,
				},
			).Do(context.Background()).Raw()

			if err != nil {
				fmt.Printf("Could not retrieve logs: %v\n", err)
			} else {
				fmt.Printf(
					"Previous container logs:\n%s\n",
					string(logs),
				)
			}

			diagnosis := controllers.DiagnoseContainer(containerStatus, string(logs))

			fmt.Printf(
				"\nDiagnosis:\n"+
					"  Problem: %s\n"+
					"  Reason: %s\n"+
					"  Severity: %s\n"+
					"  Recommended action: %s\n",
				diagnosis.Problem,
				diagnosis.Reason,
				diagnosis.Severity,
				diagnosis.Action,
			)
		}
	}
}

func getKubeConfig() (*rest.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	kubeconfig := filepath.Join(home, ".kube", "config")

	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
