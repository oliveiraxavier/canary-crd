package canary

import (
	"context"
	"fmt"
	"strings"

	"github.com/oliveiraxavier/canary-crd/api/v1alpha1"
	log "github.com/oliveiraxavier/canary-crd/internal/logs"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetStableDeployment(clientSet *client.Client, deploymentName string, namespace string) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}
	err := (*clientSet).Get(context.Background(), client.ObjectKey{Name: deploymentName, Namespace: namespace}, deployment)

	if err != nil {
		log.Custom.Info("Error fetching Stable Deployment. Verify if exists in cluster", "deployment name", deploymentName)
		return nil, err
	}

	return deployment, nil
}

func GetCanaryDeployment(clientSet *client.Client, deploymentName string, namespace string) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}
	err := (*clientSet).Get(context.Background(), client.ObjectKey{Name: deploymentName + "-canary", Namespace: namespace}, deployment)

	if err != nil {
		log.Custom.Info("Error fetching Canary Deployment, not exists", "deployment name", deploymentName+"-canary")
		return nil, err
	}
	return deployment, nil
}

func NewCanaryDeployment(clientSet *client.Client, deployment *appsv1.Deployment, appName string, deploymentCanaryVersion string) (*appsv1.Deployment, error) {
	newCanaryDeployment := deployment.DeepCopy()

	canaryLabel := map[string]string{"run-type": "canary", "app": appName}
	newCanaryDeployment.Spec.Selector.MatchLabels = canaryLabel
	newCanaryDeployment.Spec.Template.ObjectMeta.Labels = canaryLabel
	imageName := strings.Split(newCanaryDeployment.Spec.Template.Spec.Containers[0].Image, ":")
	newCanaryDeployment.Spec.Template.Spec.Containers[0].Image = imageName[0] + ":" + deploymentCanaryVersion

	deploymentCanaryExists, _ := GetCanaryDeployment(clientSet, appName, newCanaryDeployment.Namespace)

	if deploymentCanaryExists == nil {
		newCanaryDeployment.Name = appName + "-canary"
		newCanaryDeployment.ObjectMeta = metav1.ObjectMeta{Name: newCanaryDeployment.Name, Namespace: newCanaryDeployment.Namespace}

		err := (*clientSet).Create(context.Background(), newCanaryDeployment)

		if err != nil {
			log.Custom.Error(err, "Error on creation of Canary Deployment", "deployment name", newCanaryDeployment.Name)
			return nil, err
		}

		return newCanaryDeployment, nil
	}

	return nil, nil
}

func NewStableDeployment(clientSet *client.Client, deployment *appsv1.Deployment, appName string, deploymentCanary string) (*appsv1.Deployment, error) {

	newStableDeployment := deployment.DeepCopy()
	stableLabel := map[string]string{"run-type": "stable", "app": appName}
	newStableDeployment.Spec.Selector.MatchLabels = stableLabel
	newStableDeployment.Spec.Template.ObjectMeta.Labels = stableLabel
	imageName := strings.Split(newStableDeployment.Spec.Template.Spec.Containers[0].Image, ":")
	newStableDeployment.Spec.Template.Spec.Containers[0].Image = imageName[0] + ":" + deploymentCanary
	newStableDeployment.ObjectMeta = metav1.ObjectMeta{Name: newStableDeployment.Name, Namespace: newStableDeployment.Namespace}
	err := (*clientSet).Create(context.Background(), newStableDeployment)

	if err != nil {
		log.Custom.Error(err, "Error on try recreate Stable Deployment", "deployment name", newStableDeployment.Name)
		return nil, err
	}
	log.Custom.Info("Successfully recreated Stable Deployment", "stable deployment", appName)

	return newStableDeployment, nil

}

func RolloutCanaryDeploymentToStable(clientSet *client.Client, canaryDeployment *v1alpha1.CanaryDeployment, namespace string, appName string) error {

	deploymentCanary, err := GetCanaryDeployment(clientSet, appName, namespace)
	if deploymentCanary != nil {
		deployStableName := appName
		stableDeployment, _ := GetStableDeployment(clientSet, deployStableName, namespace)
		if deleteDeployment(clientSet, stableDeployment) {
			canaryLabel := map[string]string{"run-type": "stable", "app": appName}
			newDeploymentStable := deploymentCanary.DeepCopy()
			newDeploymentStable.Name = deployStableName
			newDeploymentStable.Spec.Selector.MatchLabels = canaryLabel
			newDeploymentStable.Spec.Template.ObjectMeta.Labels = canaryLabel
			newDeploymentStable.ResourceVersion = ""

			_, err := NewStableDeployment(clientSet, newDeploymentStable, deployStableName, canaryDeployment.Spec.Canary)
			if err != nil {
				return err
			}

			log.Custom.Info("Success on change canary deployment to stable deployment", "stable deployment", deployStableName)
			if deleteDeployment(clientSet, deploymentCanary) {
				err = DeleteCanaryDeployment(clientSet, canaryDeployment)
				return err
			}
			return fmt.Errorf("%s %s", "Error deleting Deployment", deploymentCanary.Name)
		}
	}

	return err
}
func deleteDeployment(clientSet *client.Client, deployment *appsv1.Deployment) bool {

	err := (*clientSet).Delete(context.Background(), deployment)
	if err != nil {
		log.Custom.Error(err, "Error deleting Deployment")
		return false
	}

	log.Custom.Info("Successfully deleted Deployment", "deployment name", deployment.Name)
	return true
}
