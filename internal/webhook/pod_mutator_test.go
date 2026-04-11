package webhook

import (
	"context"
	"encoding/json"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/defaults"
)

func schemeWithAll() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	_ = autosizev1.AddToScheme(s)
	return s
}

func TestPodMutator_Handle_injectsFromRecommendation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	scheme := schemeWithAll()
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "mydep", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
				},
			},
		},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mydep-abc",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "Deployment", Name: "mydep", UID: "1", Controller: new(true)},
			},
		},
	}
	wp := &autosizev1.WorkloadProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "wp", Namespace: "default"},
		Spec: autosizev1.WorkloadProfileSpec{
			TargetRef: autosizev1.WorkloadTargetRef{Kind: "Deployment", Name: "mydep"},
		},
		Status: autosizev1.WorkloadProfileStatus{
			Recommendations: []autosizev1.Recommendation{
				{
					ContainerName: "app",
					CPURequest:    resource.MustParse("200m"),
					CPULimit:      resource.MustParse("400m"),
					MemoryRequest: resource.MustParse("256Mi"),
					MemoryLimit:   resource.MustParse("512Mi"),
					Rationale:     "test",
				},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep, rs, wp).Build()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "mydep-abc", UID: "2", Controller: new(true)},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
		},
	}
	raw, err := json.Marshal(pod)
	if err != nil {
		t.Fatal(err)
	}
	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       "uid",
			Kind:      metav1.GroupVersionKind{Version: "v1", Kind: "Pod"},
			Operation: admissionv1.Create,
			Object:    runtime.RawExtension{Raw: raw},
		},
	}

	m := &PodMutator{
		Client:   cl,
		Decoder:  admission.NewDecoder(scheme),
		Defaults: &noopDefaults{},
	}
	resp := m.Handle(ctx, req)
	if !resp.Allowed {
		t.Fatalf("expected allowed, got %+v", resp)
	}
	if len(resp.Patches) == 0 {
		t.Fatalf("expected json patches, got %+v", resp)
	}
}

func TestPodMutator_Handle_respectsDisabledAnnotation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	scheme := schemeWithAll()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "p1",
			Namespace:   "default",
			Annotations: map[string]string{annoWebhookDisabled: valDisabled},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
	}
	raw, _ := json.Marshal(pod)
	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       "uid",
			Operation: admissionv1.Create,
			Object:    runtime.RawExtension{Raw: raw},
		},
	}
	m := &PodMutator{Client: cl, Decoder: admission.NewDecoder(scheme), Defaults: &noopDefaults{}}
	resp := m.Handle(ctx, req)
	if !resp.Allowed || len(resp.Patches) != 0 {
		t.Fatalf("expected allow without patches: %+v", resp)
	}
}

type noopDefaults struct{}

func (noopDefaults) Snapshot() *defaults.GlobalResourceDefaults { return nil }
