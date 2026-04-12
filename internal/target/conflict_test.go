package target

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

func TestDetectConflicts_NoConflict(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(scheme()).Build()

	keys := []WorkloadKey{
		{Kind: "Deployment", Namespace: "ns1", Name: "app1"},
	}
	result, err := DetectConflicts(context.Background(), c, "ns1", keys, "NamespaceProfile", "my-nsp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Free) != 1 {
		t.Fatalf("expected 1 free, got %d", len(result.Free))
	}
	if len(result.Conflicting) != 0 {
		t.Fatalf("expected 0 conflicting, got %d", len(result.Conflicting))
	}
}

func TestDetectConflicts_StandaloneWPConflict(t *testing.T) {
	existing := &autosizev1.WorkloadProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "standalone-wp", Namespace: "ns1"},
		Spec: autosizev1.WorkloadProfileSpec{
			TargetRef: autosizev1.WorkloadTargetRef{Kind: "Deployment", Name: "app1"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme()).WithObjects(existing).Build()

	keys := []WorkloadKey{
		{Kind: "Deployment", Namespace: "ns1", Name: "app1"},
		{Kind: "Deployment", Namespace: "ns1", Name: "app2"},
	}
	result, err := DetectConflicts(context.Background(), c, "ns1", keys, "NamespaceProfile", "my-nsp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Free) != 1 {
		t.Fatalf("expected 1 free, got %d", len(result.Free))
	}
	if len(result.Conflicting) != 1 {
		t.Fatalf("expected 1 conflicting, got %d", len(result.Conflicting))
	}
}

func TestDetectConflicts_OwnChildNotConflicting(t *testing.T) {
	ownChild := &autosizev1.WorkloadProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child-wp",
			Namespace: "ns1",
			Labels: map[string]string{
				autosizev1.LabelManagedBy:  "true",
				autosizev1.LabelParentKind: "NamespaceProfile",
				autosizev1.LabelParentName: "my-nsp",
			},
		},
		Spec: autosizev1.WorkloadProfileSpec{
			TargetRef: autosizev1.WorkloadTargetRef{Kind: "Deployment", Name: "app1"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme()).WithObjects(ownChild).Build()

	keys := []WorkloadKey{
		{Kind: "Deployment", Namespace: "ns1", Name: "app1"},
	}
	result, err := DetectConflicts(context.Background(), c, "ns1", keys, "NamespaceProfile", "my-nsp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Free) != 1 {
		t.Fatalf("expected 1 free (own child), got %d", len(result.Free))
	}
	if len(result.Conflicting) != 0 {
		t.Fatalf("expected 0 conflicting, got %d", len(result.Conflicting))
	}
}

func TestDetectConflicts_OtherParentConflict(t *testing.T) {
	otherChild := &autosizev1.WorkloadProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-child",
			Namespace: "ns1",
			Labels: map[string]string{
				autosizev1.LabelManagedBy:  "true",
				autosizev1.LabelParentKind: "NamespaceProfile",
				autosizev1.LabelParentName: "other-nsp",
			},
		},
		Spec: autosizev1.WorkloadProfileSpec{
			TargetRef: autosizev1.WorkloadTargetRef{Kind: "Deployment", Name: "app1"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme()).WithObjects(otherChild).Build()

	keys := []WorkloadKey{
		{Kind: "Deployment", Namespace: "ns1", Name: "app1"},
	}
	result, err := DetectConflicts(context.Background(), c, "ns1", keys, "NamespaceProfile", "my-nsp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Free) != 0 {
		t.Fatalf("expected 0 free, got %d", len(result.Free))
	}
	if len(result.Conflicting) != 1 {
		t.Fatalf("expected 1 conflicting, got %d", len(result.Conflicting))
	}
}
