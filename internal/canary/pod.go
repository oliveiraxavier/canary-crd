package canary

import (
	"context"
	"fmt"

	log "github.com/oliveiraxavier/canary-crd/internal/logs"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func allPodsRunningForDeployment(clientSet *client.Client, deployment *appsv1.Deployment, namespace string) bool {
	if deployment.Spec.Selector == nil {
		err := fmt.Errorf("deployment > spec > selector is nil for Deployment")
		log.Custom.Error(err, "deployment name", deployment.Name)
		return false
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return false
	}

	podList := &corev1.PodList{}
	listOptions := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	}

	err = (*clientSet).List(context.TODO(), podList, listOptions...)
	if err != nil {
		log.Custom.Error(err, "Error listing pods for deployment", "deployment", deployment.Name, "namespace", namespace)
		return false
	}
	allPodsRunning := 0

	for _, podItem := range podList.Items {
		log.Custom.Info("Pod", "pod name", podItem.Name, "status", podItem.Status.Phase)
		if isPodReady(&podItem) {
			allPodsRunning++
		}
	}

	totalPods := len(podList.Items)
	log.Custom.Info("Pods for deployment", "deployment", deployment.Name, "namespace", namespace, "pod_count", totalPods)
	return totalPods == allPodsRunning
}

func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func WaitAllPodsWithRunningStatus(clientSet *client.Client, deployment *appsv1.Deployment, namespace string) {
	for {
		log.Custom.Info("Waiting all new stable pods with status Running", "new deployment", deployment.Name)
		allRunning := allPodsRunningForDeployment(clientSet, deployment, namespace)
		if allRunning {
			break
		}
	}
}
