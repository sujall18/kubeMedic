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
	shouldRemediate func(Diagnosis) bool,
) error {

	fmt.Println("👀 KubeMedic watching Kubernetes...")

	// ------------------------------------------------------------
	// 1. HANDLE PODS THAT ALREADY EXIST
	// ------------------------------------------------------------
	pods, err := clientset.CoreV1().Pods("").List(
		ctx,
		metav1.ListOptions{},
	)
	if err != nil {
		return fmt.Errorf("list existing pods: %w", err)
	}

	fmt.Printf("🔎 Initial scan: found %d pods\n", len(pods.Items))

	for i := range pods.Items {
		AnalyzePod(ctx, clientset, &pods.Items[i], tracker)
	}

	// ------------------------------------------------------------
	// 2. WATCH FOR NEW CHANGES
	// ------------------------------------------------------------
	watcher, err := clientset.CoreV1().Pods("").Watch(
		ctx,
		metav1.ListOptions{},
	)
	if err != nil {
		return fmt.Errorf("watch pods: %w", err)
	}

	defer watcher.Stop()

	for {
		select {

		case <-ctx.Done():
			return ctx.Err()

		case event, ok := <-watcher.ResultChan():
			if !ok {
				return nil
			}

			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}

			switch event.Type {

			case watch.Added:
				AnalyzePod(ctx, clientset, pod, tracker)

			case watch.Modified:
				AnalyzePod(ctx, clientset, pod, tracker)

			case watch.Deleted:
				// Nothing to remediate.

			case watch.Error:
				fmt.Printf("⚠️ Kubernetes watch error: %v\n", event.Object)
			}
		}
	}
}
