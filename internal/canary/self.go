package canary

import (
	"context"

	"github.com/oliveiraxavier/canary-crd/api/v1alpha1"
	log "github.com/oliveiraxavier/canary-crd/internal/logs"
	selfutils "github.com/oliveiraxavier/canary-crd/internal/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SetSyncDate(clientSet *client.Client, canaryDeployment *v1alpha1.CanaryDeployment) (*v1alpha1.CanaryDeployment, error) {
	nextSync := selfutils.Now()
	var keyCurrentStep int32 = 0
	totalSteps := int32(len(canaryDeployment.Spec.Steps))
	if canaryDeployment.CurrentStep >= totalSteps {
		keyCurrentStep = totalSteps - 1
	} else if canaryDeployment.CurrentStep > 0 {
		keyCurrentStep = canaryDeployment.CurrentStep - 1
	}

	if canaryDeployment.Spec.Steps[keyCurrentStep].Pause.Seconds > 0 {
		stepTime := int64(canaryDeployment.Spec.Steps[keyCurrentStep].Pause.Seconds)
		nextSync = selfutils.Now().AddSeconds(stepTime)
	}

	if canaryDeployment.Spec.Steps[keyCurrentStep].Pause.Minutes > 0 {
		stepTime := int64(canaryDeployment.Spec.Steps[keyCurrentStep].Pause.Minutes)
		nextSync = selfutils.Now().AddMinutes(stepTime)
	}

	if canaryDeployment.Spec.Steps[keyCurrentStep].Pause.Hours > 0 {
		stepTime := int64(canaryDeployment.Spec.Steps[keyCurrentStep].Pause.Hours)
		nextSync = selfutils.Now().AddHours(stepTime)
	}

	originalCanary := canaryDeployment.DeepCopy()
	canaryDeployment.SyncAfter = nextSync.ToString()
	_, err := patchSelf(clientSet, originalCanary, canaryDeployment)
	return canaryDeployment, err
}

func GetRequeueTime(canaryDeployment *v1alpha1.CanaryDeployment) int64 {
	var requeueTime int64
	var keyCurrentStep int32 = 0
	appName := canaryDeployment.Spec.AppName
	totalSteps := int32(len(canaryDeployment.Spec.Steps))
	if canaryDeployment.CurrentStep >= totalSteps {
		keyCurrentStep = totalSteps - 1
	} else if canaryDeployment.CurrentStep > 0 {
		keyCurrentStep = canaryDeployment.CurrentStep - 1
	}

	if canaryDeployment.Spec.Steps[keyCurrentStep].Pause.Seconds > 0 {
		requeueTime = int64(canaryDeployment.Spec.Steps[keyCurrentStep].Pause.Seconds)
		log.Custom.Info("Canary deployment paused", "step", canaryDeployment.CurrentStep, "seconds", requeueTime, "app", appName, "current step", canaryDeployment.CurrentStep)
	}

	if canaryDeployment.Spec.Steps[keyCurrentStep].Pause.Minutes > 0 {
		requeueTime = int64(canaryDeployment.Spec.Steps[keyCurrentStep].Pause.Minutes * 60)
		log.Custom.Info("Canary deployment paused", "step", canaryDeployment.CurrentStep, "minutes", requeueTime/60, "app", appName, "current step", canaryDeployment.CurrentStep)
	}

	if canaryDeployment.Spec.Steps[keyCurrentStep].Pause.Hours > 0 {
		requeueTime = int64(canaryDeployment.Spec.Steps[keyCurrentStep].Pause.Hours * 3600)
		log.Custom.Info("Canary deployment paused", "step", canaryDeployment.CurrentStep, "hours", canaryDeployment.Spec.Steps[keyCurrentStep].Pause.Hours/3600, "app", appName, "current step", canaryDeployment.CurrentStep)
	}

	return requeueTime
}

func SetCurrentStep(clientSet *client.Client, canaryDeployment *v1alpha1.CanaryDeployment) (*v1alpha1.CanaryDeployment, error) {
	originalCanary := canaryDeployment.DeepCopy()
	totalSteps := int32(len(canaryDeployment.Spec.Steps))

	if canaryDeployment.CurrentStep > totalSteps-1 {
		canaryDeployment.CurrentStep = totalSteps
		_, err := patchSelf(clientSet, originalCanary, canaryDeployment)
		return canaryDeployment, err
	}
	canaryDeployment.CurrentStep = canaryDeployment.CurrentStep + 1
	_, err := patchSelf(clientSet, originalCanary, canaryDeployment)
	return canaryDeployment, err
}

//nolint:unparam
func patchSelf(clientSet *client.Client, originalCanary *v1alpha1.CanaryDeployment, canaryDeployment *v1alpha1.CanaryDeployment) (*v1alpha1.CanaryDeployment, error) {

	err := (*clientSet).Patch(context.Background(), canaryDeployment, client.MergeFrom(originalCanary))
	appName := canaryDeployment.Spec.AppName
	if err != nil {
		log.Custom.Error(err, "Error in update actualStep for Canary Deployment", "app", appName)
		return nil, err
	}
	return canaryDeployment, nil
}

func IsFinished(canaryDeployment v1alpha1.CanaryDeployment) bool {
	totalSteps := int32(len(canaryDeployment.Spec.Steps))
	return totalSteps == canaryDeployment.CurrentStep
}

func DeleteCanaryDeployment(clientSet *client.Client, canaryDeployment *v1alpha1.CanaryDeployment) error {
	err := (*clientSet).Delete(context.Background(), canaryDeployment)
	if err != nil {
		log.Custom.Error(err, "Error on delete Canary deployment", "canary deployment name", canaryDeployment.Spec.AppName)
		return err
	}

	log.Custom.Info("Canary deployment deleted", "canary deployment name", canaryDeployment.Spec.AppName)
	return nil
}
