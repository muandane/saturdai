package safety

import (
	"math"
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
	got := clampDecreaseMemory(newQ, cur, false)
	want := resource.MustParse("70Mi")
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
	got := clampDecreaseMemory(newQ, cur, false)
	if got.Value() < cur.Value()/2 {
		t.Fatalf("implausible result %s from cur %s new %s — possible milli/bytes mix-up", got.String(), cur.String(), newQ.String())
	}
}

func TestClampDecreaseMemory_NoClampWhenHigher(t *testing.T) {
	cur := resource.MustParse("64Mi")
	newQ := resource.MustParse("128Mi")
	got := clampDecreaseMemory(newQ, cur, false)
	if got.Cmp(newQ) != 0 {
		t.Fatalf("got %s want %s", got.String(), newQ.String())
	}
}

func TestClampDecreaseMemory_LimitUsesCeilMiB(t *testing.T) {
	cur := resource.MustParse("256Mi")
	newQ := resource.MustParse("1Mi")
	got := clampDecreaseMemory(newQ, cur, true)
	minV := int64(math.Ceil(float64(cur.Value()) * 0.7))
	want := ceilMiB(minV)
	if got.Cmp(want) != 0 {
		t.Fatalf("limit clamp got %s want %s (ceil MiB of %d)", got.String(), want.String(), minV)
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
	res := Apply(profile, base, cur, podsignals.NewSnapshot(), time.Unix(0, 0), false)
	if len(res.Recommendations) != 1 {
		t.Fatalf("len %d", len(res.Recommendations))
	}
	r := res.Recommendations[0]
	if r.CPURequest.String() != "700m" {
		t.Fatalf("CPU request got %s want 700m", r.CPURequest.String())
	}
	wantMem := resource.MustParse("70Mi")
	if r.MemoryRequest.Cmp(wantMem) != 0 {
		t.Fatalf("memory request got %s want %s", r.MemoryRequest.String(), wantMem.String())
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
	res := Apply(profile, base, cur, podsignals.NewSnapshot(), time.Unix(0, 0), false)
	r := res.Recommendations[0]
	if strings.Contains(r.Rationale, "safety: decrease_step") {
		t.Fatalf("rationale should not contain decrease_step when recommendation is above current: %q", r.Rationale)
	}
}

func TestApply_PauseDownsize_blocksDecreaseBelowCurrent(t *testing.T) {
	profile := &autosizev1.WorkloadProfile{}
	cur := map[string]corev1.ResourceRequirements{
		"app": {
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1000m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2000m"),
				corev1.ResourceMemory: resource.MustParse("200Mi"),
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
	res := Apply(profile, base, cur, podsignals.NewSnapshot(), time.Unix(0, 0), true)
	r := res.Recommendations[0]
	wantCPUReq := resource.MustParse("1000m")
	wantCPULim := resource.MustParse("2000m")
	if r.CPURequest.Cmp(wantCPUReq) != 0 || r.CPULimit.Cmp(wantCPULim) != 0 {
		t.Fatalf("cpu got request %s limit %s want 1000m / 2000m", r.CPURequest.String(), r.CPULimit.String())
	}
	wantMemReq := resource.MustParse("100Mi")
	wantMemLim := resource.MustParse("200Mi")
	if r.MemoryRequest.Cmp(wantMemReq) != 0 || r.MemoryLimit.Cmp(wantMemLim) != 0 {
		t.Fatalf("memory got request %s limit %s", r.MemoryRequest.String(), r.MemoryLimit.String())
	}
	if !strings.Contains(r.Rationale, "pause_downsize") {
		t.Fatalf("rationale should note pause_downsize: %q", r.Rationale)
	}
}

func TestApply_TrendGuard_skipsMemoryClampsAndSetsSkipMemory(t *testing.T) {
	profile := &autosizev1.WorkloadProfile{
		Status: autosizev1.WorkloadProfileStatus{
			Containers: []autosizev1.ProfileContainerStatus{
				{
					Name: "app",
					Stats: autosizev1.ContainerResourceStats{
						Memory: autosizev1.MemoryStats{SlopePositive: true},
					},
				},
			},
		},
	}
	cur := map[string]corev1.ResourceRequirements{
		"app": {
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1000m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2000m"),
				corev1.ResourceMemory: resource.MustParse("200Mi"),
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
			Rationale:     "balanced: test",
		},
	}
	res := Apply(profile, base, cur, podsignals.NewSnapshot(), time.Unix(0, 0), false)
	if !res.SkipMemory["app"] {
		t.Fatal("expected SkipMemory app=true for slopePositive")
	}
	r := res.Recommendations[0]
	if !strings.Contains(r.Rationale, "trend_guard") {
		t.Fatalf("rationale should note trend_guard: %q", r.Rationale)
	}
	// Engine memory values pass through without decrease clamp when skipMem (not lowered toward 70% floor).
	if r.MemoryRequest.Cmp(resource.MustParse("10Mi")) != 0 {
		t.Fatalf("memory request got %s want 10Mi (no clamp under trend guard)", r.MemoryRequest.String())
	}
	// Under trend guard, safe recommendations may show memory below the live template (status traceability);
	// actuation uses SkipMemory so the template is not patched down (see actuate tests).
	curMem := cur["app"].Requests[corev1.ResourceMemory]
	if r.MemoryRequest.Cmp(curMem) >= 0 {
		t.Fatalf("expected memory request below template for traceability, got %s vs template %s", r.MemoryRequest.String(), curMem.String())
	}
	// CPU still clamped.
	if r.CPURequest.String() != "700m" {
		t.Fatalf("CPU request got %s want 700m", r.CPURequest.String())
	}
}

func TestApply_PauseDownsize_allowsIncreaseAboveCurrent(t *testing.T) {
	profile := &autosizev1.WorkloadProfile{}
	cur := map[string]corev1.ResourceRequirements{
		"app": {
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
	}
	base := []autosizev1.Recommendation{
		{
			ContainerName: "app",
			CPURequest:    resource.MustParse("500m"),
			MemoryRequest: resource.MustParse("128Mi"),
			CPULimit:      resource.MustParse("1000m"),
			MemoryLimit:   resource.MustParse("256Mi"),
			Rationale:     "balanced: test",
		},
	}
	res := Apply(profile, base, cur, podsignals.NewSnapshot(), time.Unix(0, 0), true)
	r := res.Recommendations[0]
	if r.CPURequest.Cmp(resource.MustParse("500m")) != 0 {
		t.Fatalf("CPU request got %s want 500m", r.CPURequest.String())
	}
	if strings.Contains(r.Rationale, "pause_downsize") {
		t.Fatalf("rationale should not contain pause_downsize when increasing: %q", r.Rationale)
	}
}
