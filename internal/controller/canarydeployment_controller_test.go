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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
							{SetWeight: 10, Pause: appsv1alpha1.Pause{Seconds: 1}},
							{SetWeight: 10, Pause: appsv1alpha1.Pause{Seconds: 1}},
							{SetWeight: 10, Pause: appsv1alpha1.Pause{Seconds: 2}},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &appsv1alpha1.CanaryDeployment{}
			k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, resource)
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

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})

		It("should requeue if the CanaryDeployment is not found", func() {
			By("Reconciling a non-existent resource")
			controllerReconciler := &CanaryDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent-resource",
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should requeue if there is an error fetching the CanaryDeployment", func() {
			By("Reconciling a resource with an error")
			controllerReconciler := &CanaryDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Simulate an error by using an invalid namespace
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      resourceName,
					Namespace: "invalid-namespace",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle update CanaryDeployment", func() {
			By("Reconciling when update CanaryDeployment")
			controllerReconciler := &CanaryDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)

			canarydeployment.ActualStep = 2

			Expect(k8sClient.Update(ctx, canarydeployment)).To(Succeed())

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			Expect(err).NotTo(HaveOccurred())
		})

		It("should update the VirtualService percentage", func() {
			By("Reconciling a resource to update VirtualService percentage")
			controllerReconciler := &CanaryDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			k8sClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: typeNamespacedName.Namespace}, canarydeployment)
			Expect(canarydeployment).ToNot(BeNil())

			By("Validatate actual step to return equal total steps")
			canarydeployment.ActualStep = 3
			Expect(k8sClient.Update(ctx, canarydeployment)).To(Succeed())
			canary.SetActualStep(&k8sClient, canarydeployment)
			Expect(canarydeployment.ActualStep).To(Equal(int32(3)))

			totalSteps := int32(len(canarydeployment.Spec.Steps))

			By("Validatate total steps + 1 to return equal total steps")
			canarydeployment.ActualStep = totalSteps + 1
			Expect(k8sClient.Update(ctx, canarydeployment)).To(Succeed())

			canary.SetActualStep(&k8sClient, canarydeployment)
			Expect(canarydeployment.ActualStep).To(Equal(totalSteps))

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
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
