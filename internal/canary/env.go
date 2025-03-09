package canary

import (
	"github.com/oliveiraxavier/canary-crd/api/v1alpha1"
	log "github.com/oliveiraxavier/canary-crd/internal/logs"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func VerifyToAddEnvFrom(canaryDeploymentCrd *v1alpha1.CanaryDeployment, newCanaryDeployment *appsv1.Deployment) *appsv1.Deployment {

	newEnvFromSource := []corev1.EnvFromSource{}
	if len(canaryDeploymentCrd.Spec.ConfigMapNames) > 0 {
		for _, envValue := range canaryDeploymentCrd.Spec.ConfigMapNames {
			log.Custom.Info("Add configmap reference", "value", envValue, "app", newCanaryDeployment.Name)
			newEnvFromSource = append(newEnvFromSource, corev1.EnvFromSource{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: envValue,
					},
				},
			})

		}
	}
	if len(canaryDeploymentCrd.Spec.SecretNames) > 0 {
		for _, envValue := range canaryDeploymentCrd.Spec.SecretNames {
			log.Custom.Info("Add secret reference", "value", envValue, "app", newCanaryDeployment.Name)
			newEnvFromSource = append(newEnvFromSource, corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: envValue,
					},
				},
			})
		}
	}

	if len(newEnvFromSource) > 0 {
		newCanaryDeployment.Spec.Template.Spec.Containers[0].EnvFrom = newEnvFromSource
	}

	return newCanaryDeployment
}
