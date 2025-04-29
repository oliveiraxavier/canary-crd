package canary_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oliveiraxavier/canary-crd/api/v1alpha1"
	"github.com/oliveiraxavier/canary-crd/internal/canary"

	appsv1alpha1 "github.com/oliveiraxavier/canary-crd/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mockClient struct {
	client.Client
	getErr    error
	createErr error
	deleteErr error
}

func int32Ptr(i int32) *int32 {
	return &i
}
func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

func (m *mockClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object) error {
	if m.getErr != nil {
		return m.getErr
	}
	return m.Client.Get(ctx, key, obj)
}

func (m *mockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if m.createErr != nil {
		return m.createErr
	}
	return m.Client.Create(ctx, obj, opts...)
}

func (m *mockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	return m.Client.Delete(ctx, obj, opts...)
}

var _ = Describe("Test deployment", func() {
	var (
		clientSet client.Client
		mocked    *mockClient
		scheme    = runtime.NewScheme()
		_         = appsv1.AddToScheme(scheme)
		_         = v1alpha1.AddToScheme(scheme)

		defaultDeploymentName = "myapp"

		deployLabels = map[string]string{
			"run-type": "stable",
			"app":      defaultDeploymentName,
		}

		deploymentStable = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: defaultDeploymentName, Namespace: "default"},
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
								Name:  defaultDeploymentName,
								Image: defaultDeploymentName + ":1.1",
							},
						},
					},
				},
			},
		}

		typeNamespacedName = types.NamespacedName{
			Name:      deploymentStable.Name,
			Namespace: "default",
		}
		canaryDeploymentCrd = &appsv1alpha1.CanaryDeployment{}
	)

	BeforeEach(func() {
		clientSet = fake.NewClientBuilder().WithScheme(scheme).Build()
		mocked = &mockClient{Client: clientSet}
		err := clientSet.Get(context.Background(), typeNamespacedName, canaryDeploymentCrd)
		Expect(err).To(HaveOccurred())

		canaryDeploymentCrd = &appsv1alpha1.CanaryDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultDeploymentName,
				Namespace: "default",
			},
			Spec: appsv1alpha1.CanaryDeploymentSpec{
				AppName:                 deploymentStable.Name,
				Stable:                  "1.1",
				Canary:                  "1.2",
				IstioVirtualServiceName: "fake-vs",
				Steps: []appsv1alpha1.Step{
					{Weight: 10, Pause: appsv1alpha1.Pause{Seconds: 10}},
					{Weight: 20, Pause: appsv1alpha1.Pause{Seconds: 15}},
					{Weight: 50, Pause: appsv1alpha1.Pause{Seconds: 20}},
					{Weight: 100},
				},
			},
		}
		Expect(clientSet.Create(context.Background(), canaryDeploymentCrd)).To(Succeed())
	})

	Context("GetStableDeployment", func() {
		It("should return an existing stable deployment", func() {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: defaultDeploymentName, Namespace: "default"},
			}
			Expect(clientSet.Create(context.Background(), deployment)).To(Succeed())

			result, err := canary.GetStableDeployment(&mocked.Client, defaultDeploymentName, "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
		})

		It("should return an error when deployment is not found", func() {
			mocked.getErr = errors.New("not found")
			stableDeploymentResult, err := canary.GetStableDeployment(&mocked.Client, "inexistent", "default")
			Expect(err).To(HaveOccurred())
			Expect(stableDeploymentResult).To(BeNil())
		})
	})

	Context("GetCanaryDeployment", func() {
		It("should return an existing canary deployment", func() {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: defaultDeploymentName + "-canary", Namespace: "default"},
			}
			Expect(clientSet.Create(context.Background(), deployment)).To(Succeed())

			result, err := canary.GetCanaryDeployment(&mocked.Client, defaultDeploymentName, "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
		})

		It("should return an error when canary deployment is not found", func() {
			mocked.getErr = errors.New("not found")
			canaryDeploymentResult, err := canary.GetCanaryDeployment(&mocked.Client, "inexistent", "default")
			Expect(err).To(HaveOccurred())
			Expect(canaryDeploymentResult).To(BeNil())
		})
	})

	Context("NewCanaryDeployment", func() {

		// if err != nil && !cluster_errors.IsNotFound(err) {
		// }
		It("should create a new canary deployment", func() {

			Expect(clientSet.Create(context.Background(), deploymentStable)).To(Succeed())

			newDeployment, err := canary.NewCanaryDeployment(&mocked.Client, deploymentStable, canaryDeploymentCrd)
			Expect(err).ToNot(HaveOccurred())
			Expect(newDeployment).ToNot(BeNil())
			Expect(newDeployment.Name).To(Equal(defaultDeploymentName + "-canary"))
		})

		It("should return nil if deployment canary already exists", func() {
			canaryDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: defaultDeploymentName + "-canary", Namespace: "default"},
			}
			Expect(clientSet.Create(context.Background(), canaryDeployment)).To(Succeed())

			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: defaultDeploymentName + "v2-canary", Namespace: "default"},
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
									Name:  defaultDeploymentName + "v2-canary",
									Image: "myapp:1.1",
								},
							},
						},
					},
				},
			}
			newDeployment, err := canary.NewCanaryDeployment(&mocked.Client, deployment, canaryDeploymentCrd)
			Expect(err).ToNot(HaveOccurred())
			Expect(newDeployment).To(BeNil())
		})
	})
})
