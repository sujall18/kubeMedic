package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

func WatchPods(
	ctx context.Context,
	clientset kubernetes.Interface,
	tracker *IncidentTracker,
) error {

	fmt.Println("👀 KubeMedic watching Kubernetes...")

	watcher, err := clientset.CoreV1().Pods("").Watch(
		ctx,
		metav1.ListOptions{},
	)

	if err != nil {
		return err
	}

	defer watcher.Stop()

	for event := range watcher.ResultChan() {

		pod, ok := event.Object.(*corev1.Pod)

		if !ok {
			continue
		}

		switch event.Type {

		case watch.Added:
			AnalyzePod(ctx, clientset, pod, tracker)

		case watch.Modified:
			AnalyzePod(ctx, clientset, pod, tracker)
		}
	}

	return nil
}
