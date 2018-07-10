package intercom

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// Discover discovers plugins that are in a given namespace.
//
// The directory doesn't need to be absolute. For example, "." will work fine.
//
func Discover(kubeClient kubernetes.Interface, namespace string) (*corev1.ServiceList, error) {
	opts := metav1.ListOptions{
		LabelSelector: labels.Set{"plugin": "true"}.String(),
	}
	services, err := kubeClient.CoreV1().Services(namespace).List(opts)
	if err != nil {
		return nil, err
	}
	return services, nil
}
