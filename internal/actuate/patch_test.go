package actuate

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

func schemeForActuate() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	return s
}

func TestApply_Deployment_updatesPodTemplateResources(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	scheme := schemeForActuate()
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "d1", Namespace: "ns1"},
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
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()

	recs := []autosizev1.Recommendation{
		{
			ContainerName: "app",
			CPURequest:    resource.MustParse("200m"),
			CPULimit:      resource.MustParse("400m"),
			MemoryRequest: resource.MustParse("256Mi"),
			MemoryLimit:   resource.MustParse("512Mi"),
		},
	}
	if err := Apply(ctx, cl, dep, recs, nil); err != nil {
		t.Fatal(err)
	}

	got := &appsv1.Deployment{}
	if err := cl.Get(ctx, client.ObjectKeyFromObject(dep), got); err != nil {
		t.Fatal(err)
	}
	c := got.Spec.Template.Spec.Containers[0]
	if c.Resources.Requests.Cpu().String() != "200m" {
		t.Fatalf("cpu request got %s", c.Resources.Requests.Cpu().String())
	}
	if c.Resources.Limits.Cpu().String() != "400m" {
		t.Fatalf("cpu limit got %s", c.Resources.Limits.Cpu().String())
	}
	if c.Resources.Requests.Memory().String() != "256Mi" {
		t.Fatalf("mem request got %s", c.Resources.Requests.Memory().String())
	}
	if c.Resources.Limits.Memory().String() != "512Mi" {
		t.Fatalf("mem limit got %s", c.Resources.Limits.Memory().String())
	}
}

func TestApply_StatefulSet_updatesPodTemplateResources(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	scheme := schemeForActuate()
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "ns1"},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
				},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts).Build()

	recs := []autosizev1.Recommendation{
		{
			ContainerName: "app",
			CPURequest:    resource.MustParse("100m"),
			CPULimit:      resource.MustParse("200m"),
			MemoryRequest: resource.MustParse("128Mi"),
			MemoryLimit:   resource.MustParse("256Mi"),
		},
	}
	if err := Apply(ctx, cl, sts, recs, nil); err != nil {
		t.Fatal(err)
	}

	got := &appsv1.StatefulSet{}
	if err := cl.Get(ctx, client.ObjectKeyFromObject(sts), got); err != nil {
		t.Fatal(err)
	}
	c := got.Spec.Template.Spec.Containers[0]
	if c.Resources.Requests.Cpu().String() != "100m" {
		t.Fatalf("cpu request got %s", c.Resources.Requests.Cpu().String())
	}
}
