package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/sujall18/kubeMedic/controllers"
	"github.com/sujall18/kubeMedic/observability"
)

func main() {
	fmt.Println("KubeMedic starting...")
	go startObservabilityServer()

	config, err := getKubeConfig()
	if err != nil {
		panic(err)
	}

	// Give KubeMedic reasonable client-side API capacity.
	// Verification/remediation performs several API calls during a rollout.
	config.QPS = 20
	config.Burst = 40

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	err = controllers.WatchPods(
		context.Background(),
		clientset,
		controllers.NewIncidentTracker(),
		controllers.ShouldRemediate)
	if err != nil {
		panic(err)
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

func startObservabilityServer() {
	mux := http.NewServeMux()

	mux.Handle("/metrics", observability.Handler())

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	server := &http.Server{
		Addr:              ":9090",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Println("📊 Observability server listening on :9090")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("observability server error: %v\n", err)
	}
}
