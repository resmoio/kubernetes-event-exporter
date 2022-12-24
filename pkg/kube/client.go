package kube

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
)

// GetKubernetesClient returns the client if it's possible in cluster, otherwise tries to read HOME
func GetKubernetesClient() (*kubernetes.Clientset, error) {
	config, err := GetKubernetesConfig("")
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

func GetKubernetesConfig(kubeconfig string) (*rest.Config, error) {
	if len(kubeconfig) > 0 {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	// If kubeconfig is not set, try to use in cluster config.
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	} else if err != rest.ErrNotInCluster {
		return nil, err
	}

	// Read KUBECONFIG env variable as fallback
	return clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
}
