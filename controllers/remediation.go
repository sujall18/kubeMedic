package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func ShouldRemediate(diagnosis Diagnosis) bool {
	return diagnosis.Severity == "MEDIUM" || diagnosis.Severity == "HIGH"
}

func RestartPod(
	ctx context.Context,
	clientset kubernetes.Interface,
	pod *corev1.Pod,
) error {

	fmt.Printf(
		"🔧 REMEDIATION: Deleting pod %s/%s\n",
		pod.Namespace,
		pod.Name,
	)

	err := clientset.CoreV1().
		Pods(pod.Namespace).
		Delete(
			ctx,
			pod.Name,
			metav1.DeleteOptions{},
		)

	if err != nil {
		return err
	}

	fmt.Println("✅ Pod deletion requested")

	return nil
}
