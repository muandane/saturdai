package target

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

const deploymentKind = "Deployment"

func scheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = appsv1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = autosizev1.AddToScheme(s)
	return s
}

func TestListWorkloads_LabelSelector(t *testing.T) {
	dep1 := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: "ns1", Labels: map[string]string{"team": "payments"}}}
	dep2 := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "app2", Namespace: "ns1", Labels: map[string]string{"team": "billing"}}}
	dep3 := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "app3", Namespace: "ns1", Labels: map[string]string{"team": "payments"}}}

	c := fake.NewClientBuilder().WithScheme(scheme()).WithObjects(dep1, dep2, dep3).Build()
	r := NewResolver(c)

	sel := autosizev1.WorkloadSelector{
		LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "payments"}},
	}
	keys, err := r.ListWorkloads(context.Background(), "ns1", sel, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d: %v", len(keys), keys)
	}
	for _, k := range keys {
		if k.Kind != deploymentKind {
			t.Errorf("expected %s, got %s", deploymentKind, k.Kind)
		}
	}
}

func TestListWorkloads_SelectAll(t *testing.T) {
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "d1", Namespace: "ns1"}}
	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "ns1"}}

	c := fake.NewClientBuilder().WithScheme(scheme()).WithObjects(dep, sts).Build()
	r := NewResolver(c)

	sel := autosizev1.WorkloadSelector{SelectAll: true}
	keys, err := r.ListWorkloads(context.Background(), "ns1", sel, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestListWorkloads_MaxTargets(t *testing.T) {
	dep1 := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "d1", Namespace: "ns1"}}
	dep2 := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "d2", Namespace: "ns1"}}
	dep3 := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "d3", Namespace: "ns1"}}

	c := fake.NewClientBuilder().WithScheme(scheme()).WithObjects(dep1, dep2, dep3).Build()
	r := NewResolver(c)

	sel := autosizev1.WorkloadSelector{SelectAll: true}
	keys, err := r.ListWorkloads(context.Background(), "ns1", sel, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys (capped), got %d", len(keys))
	}
}

func TestListWorkloads_KindsFilter(t *testing.T) {
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "d1", Namespace: "ns1"}}
	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "ns1"}}

	c := fake.NewClientBuilder().WithScheme(scheme()).WithObjects(dep, sts).Build()
	r := NewResolver(c)

	f := false
	sel := autosizev1.WorkloadSelector{
		SelectAll: true,
		Kinds:     &autosizev1.WorkloadKinds{StatefulSet: &f},
	}
	keys, err := r.ListWorkloads(context.Background(), "ns1", sel, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key (Deployment only), got %d", len(keys))
	}
	if keys[0].Kind != deploymentKind {
		t.Errorf("expected %s, got %s", deploymentKind, keys[0].Kind)
	}
}

func TestListWorkloads_EmptySelectorWithoutSelectAll(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(scheme()).Build()
	r := NewResolver(c)

	sel := autosizev1.WorkloadSelector{}
	_, err := r.ListWorkloads(context.Background(), "ns1", sel, 50)
	if err == nil {
		t.Fatal("expected error for empty selector without selectAll")
	}
}

func TestListClusterWorkloads(t *testing.T) {
	ns1 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns1", Labels: map[string]string{"env": "prod"}}}
	ns2 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns2", Labels: map[string]string{"env": "staging"}}}
	dep1 := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "ns1"}}
	dep2 := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "ns2"}}

	c := fake.NewClientBuilder().WithScheme(scheme()).WithObjects(ns1, ns2, dep1, dep2).Build()
	r := NewResolver(c)

	nsSel := &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	wSel := autosizev1.WorkloadSelector{SelectAll: true}

	keys, err := r.ListClusterWorkloads(context.Background(), nsSel, wSel, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key (ns1 only), got %d: %v", len(keys), keys)
	}
	if keys[0].Namespace != "ns1" {
		t.Errorf("expected ns1, got %s", keys[0].Namespace)
	}
}
