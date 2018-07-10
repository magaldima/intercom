package intercom

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// ValidateDeployment for the plugin
func ValidateDeployment(d *appsv1.Deployment) error {
	// 1. only one container may be defined

	return nil
}

// ValidateService for the plugin
func ValidateService(s *corev1.Service) error {
	return nil
}
