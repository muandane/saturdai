/*
Copyright 2026.

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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/kubelet"
	"github.com/muandane/saturdai/internal/target"
)

type fakeKubelet struct{}

func (fakeKubelet) FetchSummary(_ context.Context, _ string) (*kubelet.Summary, error) {
	return &kubelet.Summary{}, nil
}

var _ = Describe("WorkloadProfile Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		workloadprofile := &autosizev1.WorkloadProfile{}

		BeforeEach(func() {
			By("creating a Deployment target")
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-workload",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test-wp"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test-wp"}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("100m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("200m"),
											corev1.ResourceMemory: resource.MustParse("256Mi"),
										},
									},
								},
							},
						},
					},
				},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: dep.Name, Namespace: dep.Namespace}, &appsv1.Deployment{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, dep)).To(Succeed())
			}

			By("creating the custom resource for the Kind WorkloadProfile")
			err = k8sClient.Get(ctx, typeNamespacedName, workloadprofile)
			if err != nil && errors.IsNotFound(err) {
				resource := &autosizev1.WorkloadProfile{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: autosizev1.WorkloadProfileSpec{
						TargetRef: autosizev1.WorkloadTargetRef{
							Kind: "Deployment",
							Name: "some-workload",
						},
						Mode: "balanced",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &autosizev1.WorkloadProfile{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance WorkloadProfile")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			dep := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "some-workload", Namespace: "default"}, dep)
			if err == nil {
				Expect(k8sClient.Delete(ctx, dep)).To(Succeed())
			}
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &WorkloadProfileReconciler{
				Client:  k8sClient,
				Scheme:  k8sClient.Scheme(),
				Target:  target.NewResolver(k8sClient),
				Kubelet: fakeKubelet{},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, typeNamespacedName, workloadprofile)).To(Succeed())
			Expect(workloadprofile.Status.MetricsRecommendations).To(HaveLen(1))
			Expect(workloadprofile.Status.Recommendations).To(HaveLen(1))
			Expect(workloadprofile.Status.Recommendations[0].Rationale).To(ContainSubstring("safety: decrease_step"))
			m := workloadprofile.Status.MetricsRecommendations[0]
			e := workloadprofile.Status.Recommendations[0]
			Expect(m.CPURequest.Cmp(e.CPURequest)).NotTo(BeZero(), "metrics vs effective CPU request should differ when safety clamps from zero baseline")
			Expect(e.Rationale).To(ContainSubstring("safety: decrease_step cpu_request"))
		})
	})
})
