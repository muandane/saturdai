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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/target"
)

var _ = Describe("NamespaceProfile Controller", func() {
	const nsName = "nsp-test"

	ctx := context.Background()

	BeforeEach(func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ns), ns)
		if errors.IsNotFound(err) {
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		}
	})

	Context("When no workloads match", func() {
		It("should set SelectorResolved=False with NoTargetsFound", func() {
			nsp := &autosizev1.NamespaceProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-nsp", Namespace: nsName},
				Spec: autosizev1.NamespaceProfileSpec{
					WorkloadSelector: autosizev1.WorkloadSelector{
						LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "nonexistent"}},
					},
					Policy: autosizev1.PolicySpec{Mode: "balanced"},
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(nsp), nsp)
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, nsp)).To(Succeed())
			}

			reconciler := &NamespaceProfileReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Target: target.NewResolver(k8sClient),
			}
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(nsp),
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nsp), nsp)).To(Succeed())
			Expect(nsp.Status.ResolvedCount).To(Equal(int32(0)))

			cond := findNSPCondition(nsp, autosizev1.ConditionTypeSelectorResolved)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("NoTargetsFound"))

			Expect(k8sClient.Delete(ctx, nsp)).To(Succeed())
		})
	})

	Context("When workloads match by label", func() {
		It("should create child WorkloadProfiles and set conditions", func() {
			replicas := int32(1)
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pay-api",
					Namespace: nsName,
					Labels:    map[string]string{"team": "payments"},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "pay-api"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "pay-api"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "busybox"}}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, dep)).To(Succeed())

			nsp := &autosizev1.NamespaceProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "payments-nsp", Namespace: nsName},
				Spec: autosizev1.NamespaceProfileSpec{
					WorkloadSelector: autosizev1.WorkloadSelector{
						LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "payments"}},
					},
					Policy: autosizev1.PolicySpec{Mode: "balanced"},
				},
			}
			Expect(k8sClient.Create(ctx, nsp)).To(Succeed())

			reconciler := &NamespaceProfileReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Target: target.NewResolver(k8sClient),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(nsp),
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nsp), nsp)).To(Succeed())
			Expect(nsp.Status.ResolvedCount).To(Equal(int32(1)))
			Expect(nsp.Status.ActiveChildren).To(Equal(int32(1)))
			Expect(nsp.Status.Children).To(HaveLen(1))

			childName := nsp.Status.Children[0].Name
			child := &autosizev1.WorkloadProfile{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: nsName, Name: childName}, child)).To(Succeed())
			Expect(child.Spec.TargetRef.Kind).To(Equal("Deployment"))
			Expect(child.Spec.TargetRef.Name).To(Equal("pay-api"))
			Expect(child.Spec.Mode).To(Equal("balanced"))
			Expect(child.Labels[autosizev1.LabelManagedBy]).To(Equal("true"))
			Expect(child.Labels[autosizev1.LabelParentKind]).To(Equal("NamespaceProfile"))

			Expect(k8sClient.Delete(ctx, nsp)).To(Succeed())
			Expect(k8sClient.Delete(ctx, dep)).To(Succeed())
		})
	})
})

func findNSPCondition(nsp *autosizev1.NamespaceProfile, typ string) *metav1.Condition {
	for i := range nsp.Status.Conditions {
		if nsp.Status.Conditions[i].Type == typ {
			return &nsp.Status.Conditions[i]
		}
	}
	return nil
}
