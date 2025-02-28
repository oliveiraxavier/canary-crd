package canary

import (
	"context"
	"strings"

	log "github.com/oliveiraxavier/canary-crd/internal/logs"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetStableDeployment returns the stable deployment for the given appName and namespace
func GetStableDeployment(clientSet *client.Client, deploymentName string, namespace string) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}
	err := (*clientSet).Get(context.Background(), client.ObjectKey{Name: deploymentName, Namespace: namespace}, deployment)

	if err != nil {
		log.Custom.Error(err, "Error fetching Stable Deployment")
		return nil, err
	}

	return deployment, nil
}

func GetCanaryDeployment(clientSet *client.Client, deploymentName string, namespace string) *appsv1.Deployment {
	deployment := &appsv1.Deployment{}
	(*clientSet).Get(context.Background(), client.ObjectKey{Name: deploymentName, Namespace: namespace}, deployment)

	return deployment
}

func NewCanaryDeployment(clientSet *client.Client, deployment *appsv1.Deployment, appName string, deploymentCanary string) (*appsv1.Deployment, error) {
	newCanaryDeployment := deployment.DeepCopy()

	canaryLabel := map[string]string{"run-type": "canary", "app": appName}
	newCanaryDeployment.Spec.Selector.MatchLabels = canaryLabel
	newCanaryDeployment.Spec.Template.ObjectMeta.Labels = canaryLabel
	imageName := strings.Split(newCanaryDeployment.Spec.Template.Spec.Containers[0].Image, ":")
	newCanaryDeployment.Spec.Template.Spec.Containers[0].Image = imageName[0] + ":" + deploymentCanary
	newCanaryDeployment.Name = newCanaryDeployment.Name + "-canary"

	deploymentCanaryExists := GetCanaryDeployment(clientSet, newCanaryDeployment.Name, newCanaryDeployment.Namespace)

	if deploymentCanaryExists.Name != "" {
		log.Custom.Info("Canary Deployment with name \"" + appName + "\", already exists")
		return nil, nil
	}

	newCanaryDeployment.ObjectMeta = metav1.ObjectMeta{Name: newCanaryDeployment.Name, Namespace: newCanaryDeployment.Namespace}

	err := (*clientSet).Create(context.Background(), newCanaryDeployment)

	if err != nil {
		log.Custom.Error(err, "Error on creation of Canary Deployment")
		return nil, err
	}

	return newCanaryDeployment, nil
}
