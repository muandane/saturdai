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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/target"
)

var _ = Describe("ClusterProfile Controller", func() {
	ctx := context.Background()

	Context("When no namespaces match", func() {
		It("should set SelectorResolved=False with NoTargetsFound", func() {
			csp := &autosizev1.ClusterProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-csp"},
				Spec: autosizev1.ClusterProfileSpec{
					NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "nonexistent"}},
					WorkloadSelector:  autosizev1.WorkloadSelector{SelectAll: true},
					Policy:            autosizev1.PolicySpec{Mode: "cost"},
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(csp), csp)
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, csp)).To(Succeed())
			}

			reconciler := &ClusterProfileReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Target: target.NewResolver(k8sClient),
			}
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(csp),
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(csp),
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(csp), csp)).To(Succeed())
			Expect(csp.Status.ResolvedCount).To(Equal(int32(0)))

			cond := findCSPCondition(csp, autosizev1.ConditionTypeSelectorResolved)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))

			Expect(k8sClient.Delete(ctx, csp)).To(Succeed())
		})
	})

	Context("When workloads exist in matching namespaces", func() {
		It("should create child WorkloadProfiles across namespaces", func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "csp-ns", Labels: map[string]string{"env": "test-csp"}}}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ns), ns)
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			}

			replicas := int32(1)
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "csp-ns"},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "web", Image: "nginx"}}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, dep)).To(Succeed())

			csp := &autosizev1.ClusterProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "test-csp"},
				Spec: autosizev1.ClusterProfileSpec{
					NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "test-csp"}},
					WorkloadSelector:  autosizev1.WorkloadSelector{SelectAll: true},
					Policy:            autosizev1.PolicySpec{Mode: "resilience"},
				},
			}
			Expect(k8sClient.Create(ctx, csp)).To(Succeed())

			reconciler := &ClusterProfileReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Target: target.NewResolver(k8sClient),
			}
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(csp),
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(csp),
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(csp), csp)).To(Succeed())
			Expect(csp.Status.ResolvedCount).To(BeNumerically(">=", int32(1)))
			Expect(csp.Status.ActiveChildren).To(BeNumerically(">=", int32(1)))
			children := &autosizev1.WorkloadProfileList{}
			Expect(k8sClient.List(ctx, children,
				client.MatchingLabels{
					autosizev1.LabelManagedBy:  "true",
					autosizev1.LabelParentKind: "ClusterProfile",
					autosizev1.LabelParentName: csp.Name,
				},
			)).To(Succeed())
			Expect(children.Items).NotTo(BeEmpty())

			Expect(k8sClient.Delete(ctx, csp)).To(Succeed())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(csp),
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(csp), &autosizev1.ClusterProfile{})
				return errors.IsNotFound(err)
			}).Should(BeTrue())

			pruned := &autosizev1.WorkloadProfileList{}
			Expect(k8sClient.List(ctx, pruned,
				client.MatchingLabels{
					autosizev1.LabelManagedBy:  "true",
					autosizev1.LabelParentKind: "ClusterProfile",
					autosizev1.LabelParentName: csp.Name,
				},
			)).To(Succeed())
			Expect(pruned.Items).To(BeEmpty())
			Expect(k8sClient.Delete(ctx, dep)).To(Succeed())
		})
	})
})

func findCSPCondition(csp *autosizev1.ClusterProfile, typ string) *metav1.Condition {
	for i := range csp.Status.Conditions {
		if csp.Status.Conditions[i].Type == typ {
			return &csp.Status.Conditions[i]
		}
	}
	return nil
}
