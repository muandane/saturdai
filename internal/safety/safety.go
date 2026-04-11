// Package safety applies cooldown, clamps, overrides, and trend guard to recommendations.
package safety

import (
	"fmt"
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	autosizev1 "github.com/muandane/saturdai/api/v1"

	"github.com/muandane/saturdai/internal/defaults"
	"github.com/muandane/saturdai/internal/podsignals"
)

// Result is the output of Apply.
type Result struct {
	Recommendations []autosizev1.Recommendation
	ShouldPatch     bool
	SkipMemory      map[string]bool
}

// Apply applies safety rules to base recommendations.
func Apply(
	profile *autosizev1.WorkloadProfile,
	base []autosizev1.Recommendation,
	current map[string]corev1.ResourceRequirements,
	sig *podsignals.Snapshot,
	now time.Time,
) Result {
	out := make([]autosizev1.Recommendation, len(base))
	copy(out, base)
	for i := range out {
		out[i].MemoryLimit = ceilMiB(out[i].MemoryLimit.Value())
		out[i].MemoryRequest = floorMiB(out[i].MemoryRequest.Value())
	}

	skipMem := map[string]bool{}
	for i := range out {
		name := out[i].ContainerName
		for _, c := range profile.Status.Containers {
			if c.Name == name && c.Stats.Memory.SlopePositive {
				skipMem[name] = true
				out[i].Rationale = out[i].Rationale + "; trend_guard: memory slope positive"
			}
		}
	}

	for i := range out {
		name := out[i].ContainerName
		if t, ok := sig.LastOOMKill[name]; ok && t != nil && now.Sub(t.Time) < 10*time.Minute {
			q := out[i].MemoryLimit
			nv := int64(math.Ceil(float64(q.Value()) * 1.5))
			out[i].MemoryLimit = ceilMiB(nv)
			out[i].Rationale = out[i].Rationale + "; override: OOMKill recent"
		}
	}

	for i := range out {
		name := out[i].ContainerName
		if r, ok := sig.ThrottleRatios[name]; ok && r > 0.5 {
			q := out[i].CPULimit
			mv := q.MilliValue()
			out[i].CPULimit = *resource.NewMilliQuantity(int64(math.Ceil(float64(mv)*1.25)), resource.DecimalSI)
			out[i].Rationale = out[i].Rationale + "; override: high CPU throttle"
		}
	}

	for i := range out {
		name := out[i].ContainerName
		cur, ok := current[name]
		if !ok {
			continue
		}
		if cur.Requests != nil {
			if c := cur.Requests.Cpu(); c != nil {
				before := out[i].CPURequest
				after := clampDecreaseCPU(before, *c)
				out[i].CPURequest = after
				out[i].Rationale = appendDecreaseStepNote(out[i].Rationale, "cpu_request", before, after, *c)
			}
			if m := cur.Requests.Memory(); m != nil && !skipMem[name] {
				before := out[i].MemoryRequest
				after := clampDecreaseMemory(before, *m, false)
				out[i].MemoryRequest = after
				out[i].Rationale = appendDecreaseStepNote(out[i].Rationale, "memory_request", before, after, *m)
			}
		}
		if cur.Limits != nil {
			if c := cur.Limits.Cpu(); c != nil {
				before := out[i].CPULimit
				after := clampDecreaseCPU(before, *c)
				out[i].CPULimit = after
				out[i].Rationale = appendDecreaseStepNote(out[i].Rationale, "cpu_limit", before, after, *c)
			}
			if m := cur.Limits.Memory(); m != nil && !skipMem[name] {
				before := out[i].MemoryLimit
				after := clampDecreaseMemory(before, *m, true)
				out[i].MemoryLimit = after
				out[i].Rationale = appendDecreaseStepNote(out[i].Rationale, "memory_limit", before, after, *m)
			}
		}
	}

	cooldown := time.Duration(defaults.Cooldown(profile.Spec)) * time.Minute

	shouldPatch := profile.Status.LastApplied == nil || now.Sub(profile.Status.LastApplied.Time) >= cooldown

	for _, t := range sig.LastOOMKill {
		if t != nil && now.Sub(t.Time) < 10*time.Minute {
			shouldPatch = true
			break
		}
	}
	for _, r := range sig.ThrottleRatios {
		if r > 0.5 {
			shouldPatch = true
			break
		}
	}

	return Result{
		Recommendations: out,
		ShouldPatch:     shouldPatch,
		SkipMemory:      skipMem,
	}
}

// appendDecreaseStepNote appends a rationale fragment when the 70% decrease clamp changes a quantity.
func appendDecreaseStepNote(rationale, axis string, before, after, current resource.Quantity) string {
	if after.Cmp(before) == 0 {
		return rationale
	}
	return rationale + fmt.Sprintf(
		"; safety: decrease_step %s %s->%s (floor 70%% of current %s)",
		axis, before.String(), after.String(), current.String(),
	)
}

// clampDecreaseCPU applies a 70% floor of current when the new recommendation is lower (millicores).
func clampDecreaseCPU(newQ, curQ resource.Quantity) resource.Quantity {
	if newQ.Cmp(curQ) >= 0 {
		return newQ
	}
	if curQ.IsZero() {
		return newQ
	}
	nm, cm := newQ.MilliValue(), curQ.MilliValue()
	if cm != 0 || nm != 0 {
		minM := int64(math.Ceil(float64(cm) * 0.7))
		if nm < minM {
			return *resource.NewMilliQuantity(minM, resource.DecimalSI)
		}
		return newQ
	}
	nv, cv := newQ.Value(), curQ.Value()
	minV := int64(math.Ceil(float64(cv) * 0.7))
	if nv < minV {
		return *resource.NewQuantity(minV, newQ.Format)
	}
	return newQ
}

// clampDecreaseMemory applies a 70% floor of current when the new recommendation is lower.
// isLimit selects ceil vs floor to whole MiB for kubectl-friendly BinarySI serialization.
func clampDecreaseMemory(newQ, curQ resource.Quantity, isLimit bool) resource.Quantity {
	if newQ.Cmp(curQ) >= 0 {
		return memoryQtyAligned(newQ.Value(), isLimit)
	}
	if curQ.IsZero() {
		return memoryQtyAligned(newQ.Value(), isLimit)
	}
	nv, cv := newQ.Value(), curQ.Value()
	minV := int64(math.Ceil(float64(cv) * 0.7))
	if nv < minV {
		return memoryQtyAligned(minV, isLimit)
	}
	return memoryQtyAligned(nv, isLimit)
}

const memoryMib = int64(1 << 20)

func ceilMiB(bytes int64) resource.Quantity {
	if bytes <= 0 {
		return *resource.NewQuantity(0, resource.BinarySI)
	}
	n := (bytes + memoryMib - 1) / memoryMib
	return *resource.NewQuantity(n*memoryMib, resource.BinarySI)
}

func floorMiB(bytes int64) resource.Quantity {
	if bytes <= 0 {
		return *resource.NewQuantity(0, resource.BinarySI)
	}
	return *resource.NewQuantity((bytes/memoryMib)*memoryMib, resource.BinarySI)
}

func memoryQtyAligned(bytes int64, isLimit bool) resource.Quantity {
	if isLimit {
		return ceilMiB(bytes)
	}
	return floorMiB(bytes)
}
