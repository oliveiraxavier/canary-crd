package canary

import (
	"context"

	"github.com/oliveiraxavier/canary-crd/api/v1alpha1"
	log "github.com/oliveiraxavier/canary-crd/internal/logs"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetRequeueTime(canaryDeployment *v1alpha1.CanaryDeployment) int64 {
	var requeueTime int64
	var keyActualStep int32 = 0

	totalSteps := int32(len(canaryDeployment.Spec.Steps))
	if canaryDeployment.ActualStep >= totalSteps {
		keyActualStep = totalSteps - 1
	} else if canaryDeployment.ActualStep > 0 {
		keyActualStep = canaryDeployment.ActualStep - 1
	}

	if canaryDeployment.Spec.Steps[keyActualStep].Pause.Seconds > 0 {
		requeueTime = int64(canaryDeployment.Spec.Steps[keyActualStep].Pause.Seconds)
	}

	if canaryDeployment.Spec.Steps[keyActualStep].Pause.Minutes > 0 {
		requeueTime = int64(canaryDeployment.Spec.Steps[keyActualStep].Pause.Minutes * 60)
	}

	if canaryDeployment.Spec.Steps[keyActualStep].Pause.Hours > 0 {
		requeueTime = int64(canaryDeployment.Spec.Steps[keyActualStep].Pause.Hours * 3600)
	}

	return requeueTime
}

func SetActualStep(clientSet *client.Client, canaryDeployment *v1alpha1.CanaryDeployment) (*v1alpha1.CanaryDeployment, error) {
	originalCanary := canaryDeployment.DeepCopy()

	totalSteps := int32(len(canaryDeployment.Spec.Steps))
	if canaryDeployment.ActualStep > totalSteps-1 {
		canaryDeployment.ActualStep = totalSteps
		patchSelf(clientSet, originalCanary, canaryDeployment)
		return canaryDeployment, nil
	}

	canaryDeployment.ActualStep = canaryDeployment.ActualStep + 1
	patchSelf(clientSet, originalCanary, canaryDeployment)

	return canaryDeployment, nil
}

func patchSelf(clientSet *client.Client, originalCanary *v1alpha1.CanaryDeployment, canaryDeployment *v1alpha1.CanaryDeployment) (*v1alpha1.CanaryDeployment, error) {

	err := (*clientSet).Patch(context.Background(), canaryDeployment, client.MergeFrom(originalCanary))

	if err != nil {
		log.Custom.Error(err, "Error in update actualStep for Canary Deployment")
		return nil, err
	}
	return canaryDeployment, nil
}

func IsFinished(canaryDeployment v1alpha1.CanaryDeployment) bool {
	totalSteps := int32(len(canaryDeployment.Spec.Steps))
	return totalSteps == canaryDeployment.ActualStep
}
