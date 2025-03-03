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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/oliveiraxavier/canary-crd/api/v1alpha1"
	"github.com/oliveiraxavier/canary-crd/internal/canary"
)

var _ = Describe("CanaryDeployment Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		canarydeployment := &appsv1alpha1.CanaryDeployment{}
		BeforeEach(func() {
			By("creating the custom resource for the Kind CanaryDeployment")
			err := k8sClient.Get(ctx, typeNamespacedName, canarydeployment)
			if err != nil && errors.IsNotFound(err) {
				resource := &appsv1alpha1.CanaryDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: appsv1alpha1.CanaryDeploymentSpec{
						AppName: "test-app",
						Stable:  "v1",
						Canary:  "v2",
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
			resource := &appsv1alpha1.CanaryDeployment{}
			// k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, resource)
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance CanaryDeployment")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
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

		It("should requeue if the CanaryDeployment is not found", func() {
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

		It("should requeue if there is an error fetching the CanaryDeployment", func() {
			By("Reconciling a resource with an error")
			controllerReconciler := &CanaryDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Simulate an error by using an invalid namespace
			ctrl, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      resourceName,
					Namespace: "invalid-namespace",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{}))
		})

		It("should handle update CanaryDeployment", func() {
			By("Reconciling when update step CanaryDeployment")
			controllerReconciler := &CanaryDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)

			canary.SetActualStep(&k8sClient, canarydeployment)
			ctrl, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			timeDuration := canary.GetRequeueTime(canarydeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Duration(timeDuration) * time.Second, Requeue: false}))

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			By("Validate time duration from pause step")
			requeueTime := int64(canarydeployment.Spec.Steps[canarydeployment.ActualStep-1].Pause.Seconds)
			timeDuration = canary.GetRequeueTime(canarydeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(timeDuration).To(Equal(requeueTime))
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Duration(timeDuration) * time.Second}))
		})

		It("should update the VirtualService percentage", func() {
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

			resource_vs := &istio.VirtualService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-app",
				},
				Spec: istiov1alpha3.VirtualService{
					Hosts: []string{"test-app"},
					Http: []*istiov1alpha3.HTTPRoute{
						{
							Route: []*istiov1alpha3.HTTPRouteDestination{
								{
									Weight: 90,
									Destination: &istiov1alpha3.Destination{
										Host:   "test-app",
										Subset: "stable",
									},
								},
								{
									Weight: 10,
									Destination: &istiov1alpha3.Destination{
										Host:   "test-app",
										Subset: "canary",
									},
								},
							},
						},
					},
				},
			}
			_, err = istioClient.NetworkingV1alpha3().VirtualServices("default").Create(ctx, resource_vs, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			// Expect(istioVs).To(Succeed())

			// vs, _ := canary.UpdateVirtualServicePercentage(&k8sClient, canarydeployment, typeNamespacedName.Namespace)
			// Expect(vs).ShouldNot(BeNil())
			// resource_vs

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			// Expect(err).NotTo(HaveOccurred())
		})

		It("should create a new CanaryDeployment if stable deployment exists", func() {
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
					AppName: "test-app",
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

	})
})
