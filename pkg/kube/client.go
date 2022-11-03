package kube

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
)

// GetKubernetesClient returns the client if its possible in cluster, otherwise tries to read HOME
func GetKubernetesClient() (*kubernetes.Clientset, error) {
	config, err := GetKubernetesConfig()
	if err != nil {
		return nil, err
	}

        // alert NewForConfig puts in a ratelimiter
	// https://github.com/kubernetes/client-go/blob/19b2e89c0c69f6993215b8547d447d87a8fc0ac7/kubernetes/clientset.go#L431
	return kubernetes.NewForConfig(config)
}

func GetKubernetesConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	} else if err != rest.ErrNotInCluster {
		return nil, err
	}

	// TODO: Read KUBECONFIG env variable as fallback
	return clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
}
