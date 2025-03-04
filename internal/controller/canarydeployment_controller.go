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
	defaultlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/oliveiraxavier/canary-crd/api/v1alpha1"
	"github.com/oliveiraxavier/canary-crd/internal/canary"
	log "github.com/oliveiraxavier/canary-crd/internal/logs"
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
	_ = defaultlog.FromContext(ctx)

	var canaryDeployment v1alpha1.CanaryDeployment

	err := r.Client.Get(ctx, req.NamespacedName, &canaryDeployment)

	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Custom.Info("Canary Deployment not found. The manifest possible deleted.")
			return ctrl.Result{}, nil
		}

		log.Custom.Info("Error fetching Canary Deployment")
		return ctrl.Result{}, nil
	}

	appName := canaryDeployment.Spec.AppName
	namespace := canaryDeployment.GetObjectMeta().GetNamespace()
	newVersion := canaryDeployment.Spec.Canary
	stableVersion := canaryDeployment.Spec.Stable

	if canary.IsFinished(canaryDeployment) {
		err = canary.RolloutCanaryDeploymentToStable(&r.Client, &canaryDeployment, namespace, appName)

		if err != nil {
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
		_, err := canary.ResetFullPercentageToStable(&r.Client, &canaryDeployment, namespace)
		if err != nil {
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
		return ctrl.Result{}, nil
	}

	stableDeployment, _ := canary.GetStableDeployment(&r.Client, appName, namespace)

	if stableDeployment != nil {
		newCanaryDeployment, err := canary.NewCanaryDeployment(&r.Client, stableDeployment, appName, newVersion)

		if newCanaryDeployment != nil && err == nil {
			log.Custom.Info("Canary Deployment created")
			log.Custom.Info("App Name: " + appName)
			log.Custom.Info("New version: " + newVersion)
			log.Custom.Info("Stable version: " + stableVersion)
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}

		canary.SetActualStep(&r.Client, &canaryDeployment)
		vs, _ := canary.UpdateVirtualServicePercentage(&r.Client, &canaryDeployment, namespace)

		if canary.IsFullyPromoted(vs) {
			log.Custom.Info("Canary deployment promoted (" + appName + ")")
			// err = canary.RolloutCanaryDeploymentToStable(&r.Client, &canaryDeployment, namespace, appName)
			// if err != nil {
			// 	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
			// }
			// return ctrl.Result{}, nil
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}

		timeDuration := canary.GetRequeueTime(&canaryDeployment)

		if timeDuration == 0 {
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}

		return ctrl.Result{RequeueAfter: time.Duration(timeDuration) * time.Second, Requeue: false}, nil
	}

	return ctrl.Result{}, nil
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
				return crdCmp
			},
		}).
		Named("canarydeployment").
		Complete(r)
}
