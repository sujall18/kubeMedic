package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
