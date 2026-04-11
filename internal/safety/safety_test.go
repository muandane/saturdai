package safety

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/podsignals"
)

func TestClampDecreaseCPU_MilliFloor(t *testing.T) {
	cur := resource.MustParse("1000m")
	newQ := resource.MustParse("100m")
	got := clampDecreaseCPU(newQ, cur)
	// 70% of 1000m = 700m
	want := resource.MustParse("700m")
	if got.Cmp(want) != 0 {
		t.Fatalf("got %s want %s", got.String(), want.String())
	}
}

func TestClampDecreaseCPU_NoClampWhenHigher(t *testing.T) {
	cur := resource.MustParse("500m")
	newQ := resource.MustParse("800m")
	got := clampDecreaseCPU(newQ, cur)
	if got.Cmp(newQ) != 0 {
		t.Fatalf("got %s want %s", got.String(), newQ.String())
	}
}

func TestClampDecreaseMemory_BytesBinarySI(t *testing.T) {
	cur := resource.MustParse("100Mi")
	newQ := resource.MustParse("10Mi")
	got := clampDecreaseMemory(newQ, cur)
	minBytes := int64(float64(cur.Value()) * 0.7)
	want := *resource.NewQuantity(minBytes, cur.Format)
	if got.Cmp(want) != 0 {
		t.Fatalf("got %s (%d) want %s (%d)", got.String(), got.Value(), want.String(), want.Value())
	}
	// Regression: must not produce DecimalSI "m" suffix for memory (milli-units).
	if got.Format != resource.BinarySI {
		t.Fatalf("memory clamp must use BinarySI, got format %v", got.Format)
	}
}

func TestClampDecreaseMemory_NotMilliScaled(t *testing.T) {
	// If we wrongly used MilliValue for memory, 100Mi would be misinterpreted and output would look like huge "m" quantities.
	cur := resource.MustParse("128Mi")
	newQ := resource.MustParse("1Mi")
	got := clampDecreaseMemory(newQ, cur)
	if got.Value() < cur.Value()/2 {
		t.Fatalf("implausible result %s from cur %s new %s — possible milli/bytes mix-up", got.String(), cur.String(), newQ.String())
	}
}

func TestClampDecreaseMemory_NoClampWhenHigher(t *testing.T) {
	cur := resource.MustParse("64Mi")
	newQ := resource.MustParse("128Mi")
	got := clampDecreaseMemory(newQ, cur)
	if got.Cmp(newQ) != 0 {
		t.Fatalf("got %s want %s", got.String(), newQ.String())
	}
}

func TestApply_UsesSeparateCPUAndMemoryClamps(t *testing.T) {
	profile := &autosizev1.WorkloadProfile{}
	cur := map[string]corev1.ResourceRequirements{
		"app": {
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1000m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
		},
	}
	base := []autosizev1.Recommendation{
		{
			ContainerName: "app",
			CPURequest:    resource.MustParse("100m"),
			MemoryRequest: resource.MustParse("10Mi"),
			CPULimit:      resource.MustParse("100m"),
			MemoryLimit:   resource.MustParse("10Mi"),
		},
	}
	res := Apply(profile, base, cur, podsignals.NewSnapshot(), time.Unix(0, 0))
	if len(res.Recommendations) != 1 {
		t.Fatalf("len %d", len(res.Recommendations))
	}
	r := res.Recommendations[0]
	if r.CPURequest.String() != "700m" {
		t.Fatalf("CPU request got %s want 700m", r.CPURequest.String())
	}
	memCur := resource.MustParse("100Mi")
	minMem := int64(float64(memCur.Value()) * 0.7)
	mr := r.MemoryRequest
	if mr.Value() != minMem {
		t.Fatalf("memory request got %d want %d", mr.Value(), minMem)
	}
	if !strings.Contains(r.Rationale, "safety: decrease_step cpu_request") {
		t.Fatalf("rationale should note cpu_request clamp: %q", r.Rationale)
	}
	if !strings.Contains(r.Rationale, "safety: decrease_step memory_request") {
		t.Fatalf("rationale should note memory_request clamp: %q", r.Rationale)
	}
}

func TestApply_NoDecreaseStepNoteWhenNoClamp(t *testing.T) {
	profile := &autosizev1.WorkloadProfile{}
	cur := map[string]corev1.ResourceRequirements{
		"app": {
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
	}
	base := []autosizev1.Recommendation{
		{
			ContainerName: "app",
			CPURequest:    resource.MustParse("200m"),
			MemoryRequest: resource.MustParse("128Mi"),
			CPULimit:      resource.MustParse("400m"),
			MemoryLimit:   resource.MustParse("256Mi"),
			Rationale:     "balanced: test",
		},
	}
	res := Apply(profile, base, cur, podsignals.NewSnapshot(), time.Unix(0, 0))
	r := res.Recommendations[0]
	if strings.Contains(r.Rationale, "safety: decrease_step") {
		t.Fatalf("rationale should not contain decrease_step when recommendation is above current: %q", r.Rationale)
	}
}
