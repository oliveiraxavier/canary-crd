/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/oliveiraxavier/canary-crd/api/v1alpha1"
	"github.com/oliveiraxavier/canary-crd/internal/canary"
	log "github.com/oliveiraxavier/canary-crd/internal/logs"
	"github.com/oliveiraxavier/canary-crd/internal/utils"
)

// CanaryDeploymentReconciler reconciles a CanaryDeployment object
type CanaryDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=mox.app.br,resources=canarydeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mox.app.br,resources=canarydeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mox.app.br,resources=canarydeployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=v1,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=v1,resources=configmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=v1,resources=configmaps/finalizers,verbs=update
// +kubebuilder:rbac:groups=v1,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=v1,resources=secrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=v1,resources=secrets/finalizers,verbs=update
func (r *CanaryDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var canaryDeploymentCrd v1alpha1.CanaryDeployment

	if err := r.Client.Get(ctx, req.NamespacedName, &canaryDeploymentCrd); err != nil {

		err := r.Client.Get(ctx, req.NamespacedName, &canaryDeploymentCrd)
		log.Custom.Info("Canary Deployment not found. The manifest possible deleted after fully upgrade.")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	appName := canaryDeploymentCrd.Spec.AppName
	namespace := canaryDeploymentCrd.GetObjectMeta().GetNamespace()
	newVersion := canaryDeploymentCrd.Spec.Canary
	stableVersion := canaryDeploymentCrd.Spec.Stable

	if stableVersion == newVersion {
		result, err := FinalizeReconcile(&r.Client, &canaryDeploymentCrd, false)
		return result, err
	}

	if canary.IsFinished(canaryDeploymentCrd) {
		err := canary.RolloutCanaryDeploymentToStable(&r.Client, &canaryDeploymentCrd, namespace, appName)

		if err != nil {
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
		_, err = canary.ResetFullPercentageToStable(&r.Client, &canaryDeploymentCrd, namespace)
		if err != nil {
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
		return ctrl.Result{}, nil
	}

	stableDeployment, _ := canary.GetStableDeployment(&r.Client, appName, namespace)

	if stableDeployment != nil {

		_, err := canary.NewCanaryDeployment(&r.Client, stableDeployment, &canaryDeploymentCrd)

		if err != nil {
			return ctrl.Result{}, err
		}

		// Prevent lose current step when restart pod
		if !utils.NowIsAfterOrEqualCompareDate(canaryDeploymentCrd.SyncAfter) {
			log.Custom.Info("Next step is after", "date", canaryDeploymentCrd.SyncAfter, "app", appName)
			timeRemaing := utils.GetTimeRemaining(canaryDeploymentCrd.SyncAfter)
			log.Custom.Info("Time remaining is", "time", timeRemaing, "app", appName)
			return ctrl.Result{RequeueAfter: timeRemaing}, nil
		}
		_, _ = canary.SetCurrentStep(&r.Client, &canaryDeploymentCrd)
		// Set next sync datetime to prevent lose current step when restart pod
		_, _ = canary.SetSyncDate(&r.Client, &canaryDeploymentCrd)
		vs, _ := canary.UpdateVirtualServicePercentage(&r.Client, &canaryDeploymentCrd, namespace)

		if canary.IsFullyPromoted(vs) {
			log.Custom.Info("Canary deployment promoted", "app", appName)
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}

		timeDuration := canary.GetRequeueTime(&canaryDeploymentCrd)

		if timeDuration == 0 {
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}

		return ctrl.Result{RequeueAfter: time.Duration(timeDuration) * time.Second}, nil

	}

	result, err := FinalizeReconcile(&r.Client, &canaryDeploymentCrd, true)
	return result, err
}

func FinalizeReconcile(clientSet *client.Client, canaryDeploymentCrd *v1alpha1.CanaryDeployment, finalizeOnly bool) (ctrl.Result, error) {

	if finalizeOnly {
		return ctrl.Result{}, nil
	}
	appName := canaryDeploymentCrd.Spec.AppName
	name := canaryDeploymentCrd.Name
	log.Custom.Info("The stable version must be different from the canary version", "stable version", canaryDeploymentCrd.Spec.Stable, "canary version", canaryDeploymentCrd.Spec.Canary)
	log.Custom.Info("Stop canary deployment", "name", name)
	err := canary.DeleteCanaryDeployment(clientSet, canaryDeploymentCrd)
	if err == nil {
		log.Custom.Info("Stop canary deployment", "name", name)
		log.Custom.Info("Canary deployment deleted. Fix manifest canary version and apply/try again", "name", name, "app", appName)
	}
	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *CanaryDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.CanaryDeployment{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldCrd := e.ObjectOld.(*v1alpha1.CanaryDeployment)
				newCrd := e.ObjectNew.(*v1alpha1.CanaryDeployment)

				crdCmp := !reflect.DeepEqual(oldCrd.CreationTimestamp, newCrd.CreationTimestamp)
				crdCmpGenerateName := !reflect.DeepEqual(oldCrd.Spec.AppName, newCrd.Spec.AppName)
				return crdCmp && crdCmpGenerateName
			},
		}).
		Named("canarydeployment").
		Complete(r)
}
