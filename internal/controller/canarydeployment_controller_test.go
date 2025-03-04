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

package controller_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	istio "istio.io/client-go/pkg/apis/networking/v1alpha3"
	ctrlRuntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/oliveiraxavier/canary-crd/api/v1alpha1"
	"github.com/oliveiraxavier/canary-crd/internal/canary"
	"github.com/oliveiraxavier/canary-crd/internal/controller"
	appsv1 "k8s.io/api/apps/v1"

	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("CanaryDeployment Controller", func() {

	const resourceName = "test-resource"
	const resourceAppName = "test-resource"
	const resourceStableVersion = "v1"
	const resourceCanaryVersion = "v2"

	ctx := context.Background()

	typeNamespacedName := types.NamespacedName{
		Name:      resourceName,
		Namespace: "default",
	}

	canarydeployment := &appsv1alpha1.CanaryDeployment{}
	stableResource := &appsv1.Deployment{}
	vsResource := &istio.VirtualService{}

	var (
		controllerReconciler controller.CanaryDeploymentReconciler
		reqReconciler        ctrlRuntime.Request
	)

	Context("When new CanaryDeployment is created on cluster", func() {

		BeforeEach(func() {
			controllerReconciler = controller.CanaryDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			reqReconciler = ctrlRuntime.Request{NamespacedName: typeNamespacedName}
			By("Find the stable deployment")
			err := k8sClient.Get(ctx, typeNamespacedName, stableResource)
			Expect(err).ToNot(BeNil())

			By("Creating the stable deployment")
			if err != nil && errors.IsNotFound(err) {
				deployLabels := map[string]string{
					"run-type": "stable",
					"app":      resourceAppName,
				}
				resourceDeployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
						Labels:    deployLabels,
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(1),
						Selector: &metav1.LabelSelector{
							MatchLabels: deployLabels,
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: deployLabels,
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  resourceName,
										Image: "nginx:" + resourceStableVersion,
									},
								},
							},
						},
					},
				}

				Expect(k8sClient.Create(ctx, resourceDeployment)).To(Succeed())
			}

			By("Creating Istio VirtualService")
			err = k8sClient.Get(ctx, typeNamespacedName, vsResource)
			Expect(err).ToNot(BeNil())

			if err != nil && errors.IsNotFound(err) {
				resourceVirtualService := &istio.VirtualService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: typeNamespacedName.Namespace,
					},
					Spec: istiov1alpha3.VirtualService{
						Hosts: []string{resourceName},
						Http: []*istiov1alpha3.HTTPRoute{
							{
								Route: []*istiov1alpha3.HTTPRouteDestination{
									{
										Weight: 90,
										Destination: &istiov1alpha3.Destination{
											Host:   resourceName,
											Subset: "stable",
										},
									},
									{
										Weight: 10,
										Destination: &istiov1alpha3.Destination{
											Host:   resourceName,
											Subset: "canary",
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resourceVirtualService)).NotTo(HaveOccurred())
			}

			By("Creating the Canary crd")
			err = k8sClient.Get(ctx, typeNamespacedName, canarydeployment)
			Expect(err).ToNot(BeNil())

			if err != nil && errors.IsNotFound(err) {

				resource := &appsv1alpha1.CanaryDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: appsv1alpha1.CanaryDeploymentSpec{
						AppName: resourceAppName,
						Stable:  resourceStableVersion,
						Canary:  resourceCanaryVersion,
						Steps: []appsv1alpha1.Step{
							{SetWeight: 10, Pause: appsv1alpha1.Pause{Seconds: 10}},
							{SetWeight: 20, Pause: appsv1alpha1.Pause{Seconds: 15}},
							{SetWeight: 50, Pause: appsv1alpha1.Pause{Seconds: 20}},
							{SetWeight: 100},
						},
					},
				}

				Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			}

		})

		AfterEach(func() {

			By("Find stableResource deployment to delete")
			err := k8sClient.Get(ctx, typeNamespacedName, stableResource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup stableResource deployment")
			Expect(k8sClient.Delete(ctx, stableResource)).To(Succeed())

			By("Find Istio VirtualService to delete")
			err = k8sClient.Get(ctx, typeNamespacedName, vsResource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup VirtualService")
			Expect(k8sClient.Delete(ctx, vsResource)).To(Succeed())

			By("Find canarydeployment crd to delete")
			err = k8sClient.Get(ctx, typeNamespacedName, canarydeployment)
			if !errors.IsNotFound(err) {
				Expect(err).NotTo(HaveOccurred())
				By("Cleanup CanaryDeployment crd")
				Expect(k8sClient.Delete(ctx, canarydeployment)).To(Succeed())
			}
		})

		It("Should successfully reconcile the resource", func() {
			By("Reconciling the created resource")

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).NotTo(HaveOccurred())

			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))
		})

		It("Should requeue if the CanaryDeployment is not found", func() {
			By("Reconciling a non-existent resource")

			ctrl, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent-resource",
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{}))

		})

		It("Should test is finished CanaryDeployment", func() {
			controllerReconciler.Reconcile(ctx, ctrlRuntime.Request{})

			By("Get canary deployment total steps")
			totalSteps := int32(len(canarydeployment.Spec.Steps))
			Expect(totalSteps).To(BeNumerically(">", 0))

			By("Get canary deployment AppName (crd)")
			appName := canarydeployment.Spec.AppName
			Expect(appName).To(Equal(resourceAppName))

			By("Get canary deployment Namespace (crd)")
			namespace := canarydeployment.GetObjectMeta().GetNamespace()
			Expect(namespace).Should(Equal(typeNamespacedName.Namespace))

			By("Get canary deployment StableVersion (crd)")
			stableVersion := canarydeployment.Spec.Stable
			Expect(stableVersion).To(Equal(resourceStableVersion))

			By("Get canary deployment CanaryVersion (crd)")
			newVersion := canarydeployment.Spec.Canary
			Expect(newVersion).To(Equal(resourceCanaryVersion))

			By("Get stable deployment (kind deployment)")
			stableDeployment, err := canary.GetStableDeployment(&k8sClient, appName, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(stableDeployment).NotTo(BeNil())

			By("Get new canary deployment (kind deployment)")
			newCanaryDeployment, err := canary.GetCanaryDeployment(&k8sClient, resourceAppName, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(newCanaryDeployment).NotTo(BeNil())
			Expect(newCanaryDeployment.Spec.Selector.MatchLabels).To(HaveKeyWithValue("run-type", "canary"))
			Expect(newCanaryDeployment.Spec.Template.ObjectMeta.Labels).To(HaveKeyWithValue("run-type", "canary"))

			setActualStep := totalSteps

			//simulate update actualStep to last
			k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)
			canarydeployment.ActualStep = setActualStep

			err = k8sClient.Update(ctx, canarydeployment)
			Expect(err).ToNot(HaveOccurred())
			Expect(canarydeployment.ActualStep).To(Equal(totalSteps))

			By("Set canarydeployment.ActualStep equals " + string(totalSteps) + "-1")
			canarydeployment.ActualStep = setActualStep
			canarydeployment, err = canary.SetActualStep(&k8sClient, canarydeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(canarydeployment.ActualStep).To(Equal(totalSteps))

			By("Verify is finished (crd)")
			isFinished := canary.IsFinished(*canarydeployment)
			Expect(isFinished).To(Equal(true))

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{}))

		})

		It("Should test fail RolloutCanaryDeploymentToStable after is finished CanaryDeployment", func() {

			k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)

			By("Get canary deployment total steps")
			totalSteps := int32(len(canarydeployment.Spec.Steps))
			Expect(totalSteps).To(BeNumerically(">", 0))

			By("Get canary deployment AppName (crd)")
			appName := canarydeployment.Spec.AppName
			Expect(appName).To(Equal(resourceAppName))

			By("Get canary deployment Namespace (crd)")
			namespace := canarydeployment.GetObjectMeta().GetNamespace()
			Expect(namespace).Should(Equal(typeNamespacedName.Namespace))

			setActualStep := totalSteps - 1
			canarydeployment.ActualStep = setActualStep

			By("Set canarydeployment.ActualStep equals " + string(totalSteps) + "-1")
			canarydeployment, err := canary.SetActualStep(&k8sClient, canarydeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(canarydeployment.ActualStep).To(Equal(totalSteps))

			By("Verify is finished (crd)")
			isFinished := canary.IsFinished(*canarydeployment)
			Expect(isFinished).To(BeTrue())

			err = canary.RolloutCanaryDeploymentToStable(&k8sClient, canarydeployment, namespace, "inexistent-app")
			Expect(err).To(HaveOccurred())

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))
		})

		It("Should test fail ResetFullPercentageToStable after is finished CanaryDeployment", func() {

			k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)

			By("Get canary deployment total steps")
			totalSteps := int32(len(canarydeployment.Spec.Steps))
			Expect(totalSteps).To(BeNumerically(">", 0))

			By("Get canary deployment AppName (crd)")
			appName := canarydeployment.Spec.AppName
			Expect(appName).To(Equal(resourceAppName))

			By("Get canary deployment Namespace (crd)")
			namespace := canarydeployment.GetObjectMeta().GetNamespace()
			Expect(namespace).Should(Equal(typeNamespacedName.Namespace))

			setActualStep := totalSteps - 1
			canarydeployment.ActualStep = setActualStep

			By("Set canarydeployment.ActualStep equals " + string(totalSteps) + "-1")
			canarydeployment, err := canary.SetActualStep(&k8sClient, canarydeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(canarydeployment.ActualStep).To(Equal(totalSteps))

			By("Verify is finished (crd)")
			isFinished := canary.IsFinished(*canarydeployment)
			Expect(isFinished).To(Equal(true))

			//TODO validate when return of canary.ResetFullPercentageToStable is nil
			// _, errReset := canary.ResetFullPercentageToStable(&k8sClient, canarydeployment, "inexistent-namespace")
			// Expect(errReset).To(HaveOccurred())

			// ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			// Expect(err).ToNot(HaveOccurred())
			// Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))

			_, errVs := canary.ResetFullPercentageToStable(&k8sClient, canarydeployment, "default")
			Expect(errVs).ToNot(HaveOccurred())

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))
		})

		It("Should handle update CanaryDeployment", func() {
			By("Reconciling when update step CanaryDeployment")

			k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)

			canarydeployment.ActualStep = 1
			By("Validate time duration from pause step")
			canarydeployment, _ := canary.SetActualStep(&k8sClient, canarydeployment)

			requeueTime := int64(canarydeployment.Spec.Steps[canarydeployment.ActualStep-1].Pause.Seconds)
			timeDuration := canary.GetRequeueTime(canarydeployment)

			Expect(timeDuration).To(Equal(requeueTime))

			controllerReconciler := &controller.CanaryDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))

		})

		It("Should update the VirtualService percentage", func() {
			By("Reconciling a resource to update VirtualService percentage")
			controllerReconciler := &controller.CanaryDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)
			Expect(err).NotTo(HaveOccurred())

			// para validação dos 20 segundos
			//20 segundos veja a linha que contém {SetWeight: 50, Pause: appsv1alpha1.Pause{Seconds: 20}}
			setActualStep := int32(1)
			canarydeployment.ActualStep = setActualStep
			Expect(k8sClient.Update(ctx, canarydeployment)).Should(Succeed())

			_, err = canary.SetActualStep(&k8sClient, canarydeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(canarydeployment.ActualStep).To(Equal(setActualStep + 1))

			By("Get Istio Virtual service")
			Expect(k8sClient.Get(ctx, typeNamespacedName, vsResource)).NotTo(HaveOccurred())

			By("Test run UpdateVirtualServicePercentage")
			vs, _ := canary.UpdateVirtualServicePercentage(&k8sClient, canarydeployment, typeNamespacedName.Namespace)
			Expect(vs).ShouldNot(BeNil())
			stepToCompare := int32(0)
			if canarydeployment.ActualStep > 0 {
				stepToCompare = canarydeployment.ActualStep - 1
			}
			Expect(vs.Spec.Http[0].Route[1].Weight).To(Equal(canarydeployment.Spec.Steps[stepToCompare].SetWeight))

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())

			//20 segundos veja a linha que contém {SetWeight: 50, Pause: appsv1alpha1.Pause{Seconds: 20}}
			Expect(ctrl).To(Equal(reconcile.Result{
				Requeue:      false,
				RequeueAfter: time.Second * time.Duration(canarydeployment.Spec.Steps[canarydeployment.ActualStep].Pause.Seconds),
			}),
			)

		})

		It("Should test is IsFullyPromoted CanaryDeployment", func() {

			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)
			Expect(err).NotTo(HaveOccurred())

			totalSteps := int32(len(canarydeployment.Spec.Steps))
			setActualStep := totalSteps
			canarydeployment.ActualStep = setActualStep
			Expect(k8sClient.Update(ctx, canarydeployment)).Should(Succeed())

			_, err = canary.SetActualStep(&k8sClient, canarydeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(canarydeployment.ActualStep).To(Equal(setActualStep))

			By("Get Istio Virtual service")
			Expect(k8sClient.Get(ctx, typeNamespacedName, vsResource)).NotTo(HaveOccurred())

			canary.SetActualStep(&k8sClient, canarydeployment)
			vs, _ := canary.UpdateVirtualServicePercentage(&k8sClient, canarydeployment, typeNamespacedName.Namespace)
			Expect(vs).ToNot(BeNil())

			By("Test if fullyPromoted")
			fullyPromoted := canary.IsFullyPromoted(vs)
			Expect(fullyPromoted).To(BeTrue())

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{}))

		})

		It("Should test time duration equal zero", func() {
			// controllerReconciler.Reconcile(ctx, reqReconciler)

			By("Get canary deployment crd")
			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)
			Expect(err).NotTo(HaveOccurred())

			By("Update ActualStep canary deployment crd")
			totalSteps := int32(len(canarydeployment.Spec.Steps))
			canarydeployment.ActualStep = totalSteps
			Expect(k8sClient.Update(ctx, canarydeployment)).Should(Succeed())
			controllerReconciler.Reconcile(ctx, reqReconciler)

			By("Call SetActualStep")
			_, err = canary.SetActualStep(&k8sClient, canarydeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(canarydeployment.ActualStep).To(Equal(totalSteps))

			By("Validate time duration")
			timeDuration := canary.GetRequeueTime(canarydeployment)
			Expect(timeDuration).To(Equal(int64(0)))

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))
		})

		It("Should test time duration not equal zero", func() {
			By("Get canary deployment crd")
			setActualStep := int32(0)
			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(canarydeployment.ActualStep).To(Equal(setActualStep))

			By("Call SetActualStep")
			canarydeployment, err = canary.SetActualStep(&k8sClient, canarydeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(canarydeployment.ActualStep).To(Equal(setActualStep + 1))

			By("Validate time duration")
			timeDuration := canary.GetRequeueTime(canarydeployment)
			Expect(timeDuration).To(BeNumerically(">", int64(0)))

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())

			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Duration(timeDuration) * time.Second, Requeue: false}))
		})

		// It("Should test RolloutCanaryDeploymentToStable and ResetFullPercentageToStable after is finished CanaryDeployment", func() {

		// 	controllerReconciler.Reconcile(ctx, reqReconciler)

		// 	k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)

		// 	By("Get canary deployment total steps")
		// 	totalSteps := int32(len(canarydeployment.Spec.Steps))
		// 	Expect(totalSteps).To(BeNumerically(">", 0))

		// 	By("Get canary deployment AppName (crd)")
		// 	appName := canarydeployment.Spec.AppName
		// 	Expect(appName).To(Equal(resourceAppName))

		// 	By("Get canary deployment Namespace (crd)")
		// 	namespace := canarydeployment.GetObjectMeta().GetNamespace()
		// 	Expect(namespace).Should(Equal(typeNamespacedName.Namespace))

		// 	setActualStep := totalSteps - 1
		// 	canarydeployment.ActualStep = setActualStep

		// 	By("Set canarydeployment.ActualStep equals " + string(totalSteps) + "-1")
		// 	canarydeployment, err := canary.SetActualStep(&k8sClient, canarydeployment)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(canarydeployment.ActualStep).To(Equal(totalSteps))

		// 	By("Verify is finished (crd)")
		// 	isFinished := canary.IsFinished(*canarydeployment)
		// 	Expect(isFinished).To(Equal(true))

		// 	err = canary.RolloutCanaryDeploymentToStable(&k8sClient, canarydeployment, namespace, appName)
		// 	Expect(err).ToNot(HaveOccurred())

		// 	vs, err := canary.ResetFullPercentageToStable(&k8sClient, canarydeployment, namespace)
		// 	Expect(err).ToNot(HaveOccurred())
		// 	Expect(vs).ToNot(BeNil())

		// 	ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
		// 	Expect(err).ToNot(HaveOccurred())
		// 	Expect(ctrl).To(Equal(reconcile.Result{}))
		// })
	})
})
