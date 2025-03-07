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

	typeNamespacedNameCanary := types.NamespacedName{
		Name:      resourceName + "-canary",
		Namespace: "default",
	}

	canarydeployment := &appsv1alpha1.CanaryDeployment{}
	stableResource := &appsv1.Deployment{}
	canaryResource := &appsv1.Deployment{}
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
			Expect(err).To(HaveOccurred())

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
			// Expect(err).ToNot(BeNil())
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
			// Expect(err).ToNot(BeNil())

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
			// Expect(err).NotTo(HaveOccurred())
			if err == nil {
				By("Cleanup stableResource deployment")
				Expect(k8sClient.Delete(ctx, stableResource)).To(Succeed())
			}

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

			By("Find canaryResource deployment to delete")
			err = k8sClient.Get(ctx, typeNamespacedNameCanary, canaryResource)
			if err == nil {
				By("Cleanup canaryResource deployment")
				Expect(k8sClient.Delete(ctx, canaryResource)).To(Succeed())
			}

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

		It("Test if CanaryDeployment is nil", func() {

			err := k8sClient.Get(ctx, client.ObjectKey{Name: "inexistent-app", Namespace: typeNamespacedName.Namespace}, canarydeployment)
			Expect(err).To(HaveOccurred())

			ctrl, err := controllerReconciler.Reconcile(ctx, reconcile.Request{})
			Expect(err).ToNot(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).To(Succeed())
			Expect(ctrl).To(Equal(reconcile.Result{}))
		})

		It("Test if CanaryDeployment is not found", func() {
			err := k8sClient.Delete(ctx, canarydeployment)
			Expect(err).ToNot(HaveOccurred())

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(ctrl).To(Equal(reconcile.Result{}))
			Expect(err).ToNot(HaveOccurred())
		})

		It("Test if Deployment stable is not found", func() {
			err := k8sClient.Get(ctx, typeNamespacedName, stableResource)
			Expect(err).ToNot(HaveOccurred())
			err = k8sClient.Delete(ctx, stableResource)
			Expect(err).ToNot(HaveOccurred())

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(ctrl).To(Equal(reconcile.Result{}))
			Expect(err).ToNot(HaveOccurred())
		})

		It("Test if canary isFinished return false and deployment not exists", func() {
			_, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Get(ctx, typeNamespacedName, stableResource)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Delete(ctx, stableResource)
			Expect(err).ToNot(HaveOccurred())

			isFinished := canary.IsFinished(*canarydeployment)
			Expect(isFinished).To(BeFalse())
			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{}))
		})

		It("Test if canary isFinished return false", func() {
			_, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())

			isFinished := canary.IsFinished(*canarydeployment)
			Expect(isFinished).To(BeFalse())

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))
		})

		It("Test if canary isFinished return true", func() {
			_, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())

			totalSteps := int32(len(canarydeployment.Spec.Steps))
			canarydeployment.CurrentStep = totalSteps - 1

			_, err = canary.SetCurrentStep(&k8sClient, canarydeployment)
			Expect(err).ToNot(HaveOccurred())

			isFinished := canary.IsFinished(*canarydeployment)
			Expect(isFinished).To(BeTrue())

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{}))
		})

		It("Test statements when isFinished return true (part 1)", func() {
			totalSteps := int32(len(canarydeployment.Spec.Steps))
			canarydeployment.CurrentStep = totalSteps - 1

			_, err := canary.SetCurrentStep(&k8sClient, canarydeployment)
			Expect(err).ToNot(HaveOccurred())

			err = canary.RolloutCanaryDeploymentToStable(&k8sClient, canarydeployment, "inexistent-namespace", "inexistent-"+resourceName)
			Expect(err).To(HaveOccurred())

			ctrl, _ := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))
		})

		It("Test statements when isFinished return true (part 2)", func() {
			totalSteps := int32(len(canarydeployment.Spec.Steps))
			canarydeployment.CurrentStep = totalSteps

			_, err := canary.SetCurrentStep(&k8sClient, canarydeployment)
			Expect(err).ToNot(HaveOccurred())

			_, err = canary.ResetFullPercentageToStable(&k8sClient, canarydeployment, "inexistent-namespace")
			Expect(err).To(HaveOccurred())
			ctrl, _ := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))
		})

		It("Test statements when isFinished return true (part 3)", func() {
			totalSteps := int32(len(canarydeployment.Spec.Steps))
			canarydeployment.CurrentStep = totalSteps - 1
			_, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())

			err = canary.RolloutCanaryDeploymentToStable(&k8sClient, canarydeployment, "default", resourceAppName)
			Expect(err).ToNot(HaveOccurred())

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(ctrl).To(Equal(reconcile.Result{}))
			Expect(err).ToNot(HaveOccurred())

		})

		It("Test statements when isFinished return true (part 4)", func() {
			totalSteps := int32(len(canarydeployment.Spec.Steps))
			canarydeployment.CurrentStep = totalSteps - 1
			_, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())

			err = canary.RolloutCanaryDeploymentToStable(&k8sClient, canarydeployment, "default", resourceAppName)
			Expect(err).ToNot(HaveOccurred())

			_, err = canary.ResetFullPercentageToStable(&k8sClient, canarydeployment, "default")
			Expect(err).ToNot(HaveOccurred())

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(ctrl).To(Equal(reconcile.Result{}))
			Expect(err).ToNot(HaveOccurred())

		})

		It("Test statements for inexistent Stable deployment ", func() {
			_, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())

			appName := canarydeployment.Spec.AppName
			_, err = canary.GetStableDeployment(&k8sClient, "inexistent-"+appName, "default")
			Expect(errors.IsNotFound(err)).To(BeTrue())

			ctrl, _ := controllerReconciler.Reconcile(ctx, reconcile.Request{})
			Expect(ctrl).To(Equal(reconcile.Result{}))
		})

		It("Test statements for Stable deployment ", func() {
			// controllerReconciler.Reconcile(ctx, reqReconciler)
			appName := canarydeployment.Spec.AppName
			newVersion := canarydeployment.Spec.Canary
			stableDeployment, _ := canary.GetStableDeployment(&k8sClient, appName, "default")
			Expect(stableDeployment).ToNot(BeNil())

			newCanaryDeployment, err := canary.NewCanaryDeployment(&k8sClient, stableDeployment, appName, newVersion)
			Expect(newCanaryDeployment).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())

			ctrl, _ := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))

		})

		It("Test statements for isFullyPromoted equal false", func() {

			canarydeployment.CurrentStep = 1
			vs, err := canary.UpdateVirtualServicePercentage(&k8sClient, canarydeployment, "default")
			Expect(err).ToNot(HaveOccurred())

			_, err = controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())

			isFullyPromoted := canary.IsFullyPromoted(vs)
			Expect(isFullyPromoted).To(BeFalse())

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))

		})

		It("Test statements for isFullyPromoted equal true", func() {
			totalSteps := int32(len(canarydeployment.Spec.Steps))
			canarydeployment.CurrentStep = totalSteps

			vs, err := canary.UpdateVirtualServicePercentage(&k8sClient, canarydeployment, "default")
			Expect(err).ToNot(HaveOccurred())

			isFullyPromoted := canary.IsFullyPromoted(vs)
			Expect(isFullyPromoted).To(BeTrue())

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))
			Expect(err).ToNot(HaveOccurred())
		})

		It("Test timeDuration when isFullyPromoted equal true", func() {
			totalSteps := int32(len(canarydeployment.Spec.Steps))
			canarydeployment.CurrentStep = totalSteps

			timeDuration := canary.GetRequeueTime(canarydeployment)
			Expect(timeDuration).To(BeNumerically("==", int64(0)))

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)

			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))
			Expect(err).ToNot(HaveOccurred())
		})

		It("Test timeDuration only", func() {
			canarydeployment.CurrentStep = 4

			timeDuration := canary.GetRequeueTime(canarydeployment)
			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(timeDuration).To(Equal(int64(0)))
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))
			Expect(err).ToNot(HaveOccurred())
		})

		// It("Test timeDuration when isFullyPromoted not equal true", func() {
		// 	_, err := canary.SetCurrentStep(&k8sClient, canarydeployment)
		// 	Expect(err).ToNot(HaveOccurred())

		// 	timeDuration := canary.GetRequeueTime(canarydeployment)

		// 	ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
		// 	Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Duration(timeDuration) * time.Second, Requeue: false}))
		// 	Expect(err).ToNot(HaveOccurred())
		// })

	})
})
