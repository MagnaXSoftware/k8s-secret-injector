package kube

import (
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"time"
)

// NewClient creates a new Kubernetes REST client.
// apiserverHost is the URL of the API server, kubeConfigFile is the location
// of a kubeconfig file. If defined, the kubeconfig file is loaded first, the
// URL of the API server read from the file is then optionally overridden by
// the value of apiserverHost.
// This function was taken from k8s' ingress-nginx
func NewClient(apiserverHost, kubeConfigFile string) (*kubernetes.Clientset, error) {
	cfg, err := clientcmd.BuildConfigFromFlags(apiserverHost, kubeConfigFile)
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	var v *version.Info

	defaultRetry := wait.Backoff{
		Steps:    3,
		Duration: 1 * time.Second,
		Factor:   1.5,
		Jitter:   0.1,
	}

	var lastErr error
	retries := 0
	klog.V(2).Info("Trying to discover Kubernetes version")
	err = wait.ExponentialBackoff(defaultRetry, func() (bool, error) {
		v, err = client.Discovery().ServerVersion()

		if err == nil {
			return true, nil
		}

		lastErr = err
		klog.V(2).Infof("Unexpected error discovering Kubernetes version (attempt %v): %v", retries, err)
		retries++
		return false, nil
	})

	// err is returned in case of timeout in the exponential backoff (ErrWaitTimeout)
	if err != nil {
		return nil, lastErr
	}

	// this should not happen, warn the user
	if retries > 0 {
		klog.Warningf("Initial connection to the Kubernetes API server was retried %d times.", retries)
	}

	klog.Infof("Running in Kubernetes cluster version v%v.%v (%v) - git (%v) commit %v - platform %v",
		v.Major, v.Minor, v.GitVersion, v.GitTreeState, v.GitCommit, v.Platform)

	return client, nil
}
