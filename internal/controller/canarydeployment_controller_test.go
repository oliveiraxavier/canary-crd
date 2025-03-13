package controller_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	istio "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrlRuntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/oliveiraxavier/canary-crd/api/v1alpha1"
	"github.com/oliveiraxavier/canary-crd/internal/canary"
	"github.com/oliveiraxavier/canary-crd/internal/controller"
	"github.com/oliveiraxavier/canary-crd/internal/utils"
	appsv1 "k8s.io/api/apps/v1"

	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("CanaryDeployment Controller", func() {
	var (
		resourceName          = "test-resource"
		resourceAppName       = "test-resource"
		resourceStableVersion = "v1"
		resourceCanaryVersion = "v2"

		ctx = context.Background()

		typeNamespacedName = types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		typeNamespacedNameCanary = types.NamespacedName{
			Name:      resourceName + "-canary",
			Namespace: "default",
		}
		canarydeployment = &appsv1alpha1.CanaryDeployment{}
		stableResource   = &appsv1.Deployment{}
		canaryResource   = &appsv1.Deployment{}
		vsResource       = &istio.VirtualService{}

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
						AppName:                 resourceAppName,
						Stable:                  resourceStableVersion,
						Canary:                  resourceCanaryVersion,
						IstioVirtualServiceName: resourceName,
						Steps: []appsv1alpha1.Step{
							{SetWeight: 10, Pause: appsv1alpha1.Pause{Seconds: 17}},
							{SetWeight: 20, Pause: appsv1alpha1.Pause{Seconds: 22}},
							{SetWeight: 50, Pause: appsv1alpha1.Pause{Seconds: 27}},
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
			if !errors.IsNotFound(err) {
				By("Cleanup canaryResource deployment")
				Expect(k8sClient.Delete(ctx, canaryResource)).To(Succeed())
			}

		})

		It("Should stop reconcile when CanaryDeployment CRD is not found", func() {
			err := k8sClient.Get(ctx, typeNamespacedName, canarydeployment)
			Expect(err).ToNot(HaveOccurred())

			Expect(k8sClient.Delete(ctx, canarydeployment)).To(Succeed())

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).NotTo(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{}))

		})

		It("Test stableVersion equals newVersion", func() {
			stableVersion := canarydeployment.Spec.Stable
			canarydeployment.Spec.Canary = stableVersion
			newVersion := canarydeployment.Spec.Canary
			Expect(stableVersion).To(Equal(newVersion))

		})

		It("should return requeue after if SyncAfter is in the future", func() {
			_, _ = controllerReconciler.Reconcile(ctx, reqReconciler)

			originalGetTimeRemaining := utils.GetTimeRemaining
			defer func() { utils.GetTimeRemaining = originalGetTimeRemaining }()

			defaultDateTime := utils.Now().ToString()
			nowTime := utils.Now().Time
			timeFormat := time.RFC3339
			parsedTime, _ := time.Parse(timeFormat, defaultDateTime)

			// mock function GetTimeRemaining
			utils.GetTimeRemaining = func(futureDateToCompare string) time.Duration {
				return parsedTime.Sub(nowTime)
			}

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).ToNot(HaveOccurred())
			Expect(ctrl.RequeueAfter).To(Equal(parsedTime.Sub(nowTime)))

		})

		It("Test if now is after or equal compare datetime", func() {
			_, _ = controllerReconciler.Reconcile(ctx, reqReconciler)

			hasTimeRemaining := !utils.NowIsAfterOrEqualCompareDate(utils.Now().AddSeconds(20).ToString())
			Expect(hasTimeRemaining).To(BeTrue())
		})

		It("Should stop reconcile when Stable Deployment is not found", func() {
			err := k8sClient.Get(ctx, typeNamespacedName, stableResource)
			Expect(err).ToNot(HaveOccurred())

			Expect(k8sClient.Delete(ctx, stableResource)).To(Succeed())

			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).NotTo(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{}))

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

		It("Test IsFullyPromoted", func() {
			totalSteps := int32(len(canarydeployment.Spec.Steps))
			canarydeployment.CurrentStep = totalSteps

			vs, _ := canary.UpdateVirtualServicePercentage(&k8sClient, canarydeployment, "default")
			Expect(canary.IsFullyPromoted(vs)).To(BeTrue())

		})

	})
})

var _ = Describe("CanaryDeployment Controller with stable version equal canary", func() {

	Context("When new CanaryDeployment is created on cluster", func() {
		var (
			resourceName          = "test-resource-v2"
			resourceAppName       = "test-resource-v2"
			resourceStableVersion = "v1"
			ctx                   = context.Background()
			typeNamespacedName    = types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}
			typeNamespacedNameCanary = types.NamespacedName{
				Name:      resourceName + "-canary",
				Namespace: "default",
			}
			canarydeployment     = &appsv1alpha1.CanaryDeployment{}
			stableResource       = &appsv1.Deployment{}
			canaryResource       = &appsv1.Deployment{}
			vsResource           = &istio.VirtualService{}
			controllerReconciler controller.CanaryDeploymentReconciler
			reqReconciler        ctrlRuntime.Request
		)

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

			By("Creating the Canary crd with canary version equals stable version")
			err = k8sClient.Get(ctx, typeNamespacedName, canarydeployment)
			Expect(err).To(HaveOccurred())

			if err != nil && errors.IsNotFound(err) {
				resource := &appsv1alpha1.CanaryDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: appsv1alpha1.CanaryDeploymentSpec{
						AppName:                 resourceAppName,
						Stable:                  resourceStableVersion,
						Canary:                  resourceStableVersion,
						IstioVirtualServiceName: resourceName,
						Steps: []appsv1alpha1.Step{
							{SetWeight: 10, Pause: appsv1alpha1.Pause{Seconds: 10}},
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
			if !errors.IsNotFound(err) {
				By("Cleanup canaryResource deployment")
				Expect(k8sClient.Delete(ctx, canaryResource)).To(Succeed())
			}

		})

		It("Test FinalizeReconcile when stableVersion equals newVersion ", func() {
			_, _ = controllerReconciler.Reconcile(ctx, reqReconciler)

			ctrl, _ := controller.FinalizeReconcile(&k8sClient, canarydeployment, false)
			Expect(ctrl).To(Equal(reconcile.Result{}))

		})
	})
})

var _ = Describe("CanaryDeployment Controller in penultimate step", func() {

	Context("When Istio Virtual Service percentage Canary is 100% ", func() {
		var (
			resourceName          = "test-resource-v2"
			resourceAppName       = "test-resource-v2"
			resourceStableVersion = "v1"
			resourceCanaryVersion = "v2"
			ctx                   = context.Background()
			typeNamespacedName    = types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}
			typeNamespacedNameCanary = types.NamespacedName{
				Name:      resourceName + "-canary",
				Namespace: "default",
			}
			canarydeployment     = &appsv1alpha1.CanaryDeployment{}
			stableResource       = &appsv1.Deployment{}
			canaryResource       = &appsv1.Deployment{}
			vsResource           = &istio.VirtualService{}
			controllerReconciler controller.CanaryDeploymentReconciler
			reqReconciler        ctrlRuntime.Request
		)

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

			By("Creating Istio VirtualService canary 50/50")
			err = k8sClient.Get(ctx, typeNamespacedName, vsResource)

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
										Weight: 50,
										Destination: &istiov1alpha3.Destination{
											Host:   resourceName,
											Subset: "stable",
										},
									},
									{
										Weight: 50,
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

			By("Creating the Canary crd with canary version equals stable version")
			err = k8sClient.Get(ctx, typeNamespacedName, canarydeployment)
			Expect(err).To(HaveOccurred())

			if err != nil && errors.IsNotFound(err) {
				resource := &appsv1alpha1.CanaryDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					CurrentStep: 2,
					Spec: appsv1alpha1.CanaryDeploymentSpec{
						AppName:                 resourceAppName,
						Stable:                  resourceStableVersion,
						Canary:                  resourceCanaryVersion,
						IstioVirtualServiceName: resourceName,
						Steps: []appsv1alpha1.Step{
							{SetWeight: 10, Pause: appsv1alpha1.Pause{Seconds: 12}},
							{SetWeight: 50, Pause: appsv1alpha1.Pause{Seconds: 22}},
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
			if !errors.IsNotFound(err) {
				By("Cleanup canaryResource deployment")
				Expect(k8sClient.Delete(ctx, canaryResource)).To(Succeed())
			}

		})

		It("Test Reconcile after is fully promoted", func() {
			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).NotTo(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))

		})
	})
})

var _ = Describe("CanaryDeployment Controller in penultimate step", func() {

	Context("When Istio Virtual Service percentage Canary is 100% ", func() {
		var (
			resourceName          = "test-resource-v2"
			resourceAppName       = "test-resource-v2"
			resourceStableVersion = "v1"
			resourceCanaryVersion = "v2"
			ctx                   = context.Background()
			typeNamespacedName    = types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}
			typeNamespacedNameCanary = types.NamespacedName{
				Name:      resourceName + "-canary",
				Namespace: "default",
			}
			canarydeployment     = &appsv1alpha1.CanaryDeployment{}
			stableResource       = &appsv1.Deployment{}
			canaryResource       = &appsv1.Deployment{}
			vsResource           = &istio.VirtualService{}
			controllerReconciler controller.CanaryDeploymentReconciler
			reqReconciler        ctrlRuntime.Request
		)

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

			By("Creating Istio VirtualService canary 50/50")
			err = k8sClient.Get(ctx, typeNamespacedName, vsResource)

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
										Weight: 50,
										Destination: &istiov1alpha3.Destination{
											Host:   resourceName,
											Subset: "stable",
										},
									},
									{
										Weight: 50,
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

			By("Creating the Canary crd with canary version equals stable version")
			err = k8sClient.Get(ctx, typeNamespacedName, canarydeployment)
			Expect(err).To(HaveOccurred())

			if err != nil && errors.IsNotFound(err) {
				resource := &appsv1alpha1.CanaryDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					CurrentStep: 1,
					Spec: appsv1alpha1.CanaryDeploymentSpec{
						AppName:                 resourceAppName,
						Stable:                  resourceStableVersion,
						Canary:                  resourceCanaryVersion,
						IstioVirtualServiceName: resourceName,
						Steps: []appsv1alpha1.Step{
							{SetWeight: 10, Pause: appsv1alpha1.Pause{Seconds: 12}},
							{SetWeight: 50, Pause: appsv1alpha1.Pause{Seconds: 0}},
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
			if !errors.IsNotFound(err) {
				By("Cleanup canaryResource deployment")
				Expect(k8sClient.Delete(ctx, canaryResource)).To(Succeed())
			}

		})

		It("Test timeDuration", func() {
			ctrl, err := controllerReconciler.Reconcile(ctx, reqReconciler)
			Expect(err).NotTo(HaveOccurred())
			Expect(ctrl).To(Equal(reconcile.Result{RequeueAfter: time.Second * 10}))
		})
	})
})
