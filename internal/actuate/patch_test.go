package actuate

import (
	"context"
	"errors"
	"testing"

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
	return s
}

func TestApply_resizesPodInPlace(t *testing.T) {
	ctx := context.Background()
	scheme := schemeForActuate()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "ns1"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
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
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

	oldResize := resizePod
	t.Cleanup(func() {
		resizePod = oldResize
	})
	resizePod = func(ctx context.Context, c client.Client, resized *corev1.Pod) error {
		return c.Update(ctx, resized)
	}

	recs := []autosizev1.Recommendation{
		{
			ContainerName: "app",
			CPURequest:    resource.MustParse("200m"),
			CPULimit:      resource.MustParse("400m"),
			MemoryRequest: resource.MustParse("256Mi"),
			MemoryLimit:   resource.MustParse("512Mi"),
		},
	}
	if _, err := Apply(ctx, cl, []corev1.Pod{*pod}, recs, nil); err != nil {
		t.Fatal(err)
	}

	got := &corev1.Pod{}
	if err := cl.Get(ctx, client.ObjectKeyFromObject(pod), got); err != nil {
		t.Fatal(err)
	}
	c := got.Spec.Containers[0]
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

func TestApply_skipMemory_leavesPodMemoryUnchanged(t *testing.T) {
	ctx := context.Background()
	scheme := schemeForActuate()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "ns1"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
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
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

	oldResize := resizePod
	t.Cleanup(func() {
		resizePod = oldResize
	})
	resizePod = func(ctx context.Context, c client.Client, resized *corev1.Pod) error {
		return c.Update(ctx, resized)
	}

	recs := []autosizev1.Recommendation{
		{
			ContainerName: "app",
			CPURequest:    resource.MustParse("150m"),
			CPULimit:      resource.MustParse("300m"),
			MemoryRequest: resource.MustParse("64Mi"),
			MemoryLimit:   resource.MustParse("128Mi"),
		},
	}
	if _, err := Apply(ctx, cl, []corev1.Pod{*pod}, recs, map[string]bool{"app": true}); err != nil {
		t.Fatal(err)
	}

	got := &corev1.Pod{}
	if err := cl.Get(ctx, client.ObjectKeyFromObject(pod), got); err != nil {
		t.Fatal(err)
	}
	c := got.Spec.Containers[0]
	if c.Resources.Requests.Cpu().String() != "150m" {
		t.Fatalf("cpu request got %s want 150m", c.Resources.Requests.Cpu().String())
	}
	if c.Resources.Requests.Memory().String() != "128Mi" {
		t.Fatalf("memory request should stay unchanged when skipMemory: got %s", c.Resources.Requests.Memory().String())
	}
	if c.Resources.Limits.Memory().String() != "256Mi" {
		t.Fatalf("memory limit should stay unchanged when skipMemory: got %s", c.Resources.Limits.Memory().String())
	}
}

func TestApply_noChanges_skipsResizeCall(t *testing.T) {
	ctx := context.Background()
	scheme := schemeForActuate()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "ns1"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
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
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

	oldResize := resizePod
	t.Cleanup(func() {
		resizePod = oldResize
	})
	calls := 0
	resizePod = func(ctx context.Context, c client.Client, resized *corev1.Pod) error {
		calls++
		return nil
	}

	recs := []autosizev1.Recommendation{
		{
			ContainerName: "app",
			CPURequest:    resource.MustParse("100m"),
			CPULimit:      resource.MustParse("200m"),
			MemoryRequest: resource.MustParse("128Mi"),
			MemoryLimit:   resource.MustParse("256Mi"),
		},
	}
	if _, err := Apply(ctx, cl, []corev1.Pod{*pod}, recs, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("resize should be skipped when no resource diff, calls=%d", calls)
	}
}

func TestApply_partialFailureReportsCounts(t *testing.T) {
	ctx := context.Background()
	scheme := schemeForActuate()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "ns1"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
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
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

	oldResize := resizePod
	t.Cleanup(func() {
		resizePod = oldResize
	})
	resizePod = func(ctx context.Context, c client.Client, resized *corev1.Pod) error {
		return errors.New("resize rejected")
	}

	recs := []autosizev1.Recommendation{
		{
			ContainerName: "app",
			CPURequest:    resource.MustParse("200m"),
			CPULimit:      resource.MustParse("300m"),
			MemoryRequest: resource.MustParse("192Mi"),
			MemoryLimit:   resource.MustParse("384Mi"),
		},
	}
	result, err := Apply(ctx, cl, []corev1.Pod{*pod}, recs, nil)
	if err == nil {
		t.Fatal("expected partial failure error")
	}
	if result.Failed != 1 || result.Resized != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}
