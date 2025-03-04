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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	istio "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/oliveiraxavier/canary-crd/api/v1alpha1"
	"github.com/oliveiraxavier/canary-crd/internal/canary"
	appsv1 "k8s.io/api/apps/v1"

	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("CanaryDeployment Controller", func() {
	Context("When reconciling a resource", func() {
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

		BeforeEach(func() {
			By("Find the stable deployment")
			err := k8sClient.Get(ctx, typeNamespacedName, stableResource)
			Expect(err).ToNot(Equal(nil))

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
			Expect(err).ToNot(Equal(nil))

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
			Expect(err).ToNot(Equal(nil))

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
			k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)
			err = k8sClient.Get(ctx, typeNamespacedName, canarydeployment)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup CanaryDeployment crd")
			Expect(k8sClient.Delete(ctx, canarydeployment)).To(Succeed())

		})

		It("Should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &CanaryDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			ctrl, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))
		})

		It("Should requeue if the CanaryDeployment is not found", func() {
			By("Reconciling a non-existent resource")
			controllerReconciler := &CanaryDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

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

			controllerReconciler := &CanaryDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

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

			By("Get canary deployment StableVersion (crd)")
			stableVersion := canarydeployment.Spec.Stable
			Expect(stableVersion).To(Equal(resourceStableVersion))

			By("Get canary deployment CanaryVersion (crd)")
			newVersion := canarydeployment.Spec.Canary
			Expect(newVersion).To(Equal(resourceCanaryVersion))

			By("Get stable deployment")
			stableDeployment, err := canary.GetStableDeployment(&k8sClient, appName, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(stableDeployment).NotTo(Equal(nil))

			By("Get new canary deployment")
			newCanaryDeployment, err := canary.GetCanaryDeployment(&k8sClient, resourceAppName, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(newCanaryDeployment).NotTo(Equal(nil))
			Expect(newCanaryDeployment.Spec.Selector.MatchLabels).To(HaveKeyWithValue("run-type", "canary"))
			Expect(newCanaryDeployment.Spec.Template.ObjectMeta.Labels).To(HaveKeyWithValue("run-type", "canary"))

			By("Reconcile after create new canary deployment")
			ctrl, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))

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
			// Expect(canarydeployment.ActualStep).To(Equal(setActualStep + 1))
			Expect(canarydeployment.ActualStep).To(Equal(totalSteps))

			By("Reconciling when is finished is true")
			isFinished := canary.IsFinished(*canarydeployment)
			Expect(isFinished).To(Equal(true))

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

		})

		It("Should update the VirtualService percentage", func() {
			By("Reconciling a resource to update VirtualService percentage")
			controllerReconciler := &CanaryDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)
			Expect(err).NotTo(HaveOccurred())

			// totalSteps := int32(len(canarydeployment.Spec.Steps))

			setActualStep := int32(1)
			canarydeployment.ActualStep = setActualStep
			Expect(k8sClient.Update(ctx, canarydeployment)).Should(Succeed())

			_, err = canary.SetActualStep(&k8sClient, canarydeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(canarydeployment.ActualStep).To(Equal(setActualStep + 1))

			By("Get Istio Virtual service")
			Expect(k8sClient.Get(ctx, typeNamespacedName, vsResource)).NotTo(HaveOccurred())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Test run UpdateVirtualServicePercentage")
			vs, _ := canary.UpdateVirtualServicePercentage(&k8sClient, canarydeployment, typeNamespacedName.Namespace)
			Expect(vs).ShouldNot(BeNil())
			stepToCompare := int32(0)
			if canarydeployment.ActualStep > 0 {
				stepToCompare = canarydeployment.ActualStep - 1
			}
			Expect(vs.Spec.Http[0].Route[1].Weight).To(Equal(canarydeployment.Spec.Steps[stepToCompare].SetWeight))

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should create a new CanaryDeployment if stable deployment exists", func() {
			By("Reconciling a resource with a stable deployment")
			controllerReconciler := &CanaryDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			stableDeployment := &appsv1alpha1.CanaryDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "stable-deployment",
					Namespace: "default",
				},
				Spec: appsv1alpha1.CanaryDeploymentSpec{
					AppName: "test-resource",
					Stable:  "v1",
					Canary:  "v2",
					Steps: []appsv1alpha1.Step{
						{SetWeight: 10, Pause: appsv1alpha1.Pause{Seconds: 1}},
						{SetWeight: 10, Pause: appsv1alpha1.Pause{Seconds: 1}},
						{SetWeight: 10, Pause: appsv1alpha1.Pause{Seconds: 2}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, stableDeployment)).To(Succeed())

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
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
			Expect(vs).ToNot(Equal(nil))

			By("Test if fullyPromoted")
			fullyPromoted := canary.IsFullyPromoted(vs)
			Expect(fullyPromoted).To(BeTrue())

		})

		It("Should test is IsFullyPromoted CanaryDeployment", func() {
			By("Get canary deployment crd")
			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)
			Expect(err).NotTo(HaveOccurred())

			By("Update ActualStep canary deployment crd")
			totalSteps := int32(len(canarydeployment.Spec.Steps))
			canarydeployment.ActualStep = totalSteps
			Expect(k8sClient.Update(ctx, canarydeployment)).Should(Succeed())

			By("Call SetActualStep")
			canarydeployment, err := canary.SetActualStep(&k8sClient, canarydeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(canarydeployment.ActualStep).To(Equal(totalSteps))

			By("Validate timeduration")
			timeDuration := canary.GetRequeueTime(canarydeployment)
			Expect(timeDuration).To(Equal(int64(0)))
		})
	})
})
