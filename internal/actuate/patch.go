// Package actuation applies recommended resources to running Pods in place.
package actuate

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

var resizePod = func(ctx context.Context, c client.Client, pod *corev1.Pod) error {
	return c.SubResource("resize").Update(ctx, pod)
}

var getPod = func(ctx context.Context, c client.Client, key client.ObjectKey, pod *corev1.Pod) error {
	return c.Get(ctx, key, pod)
}

// Result summarizes actuation results for a reconcile loop.
type Result struct {
	Resized               int
	Noop                  int
	Failed                int
	ReasonCounts          map[string]int
	RestartPolicyWarnings int
}

// Apply updates running Pod container resources from recommendations using the pod/resize subresource.
func Apply(ctx context.Context, c client.Client, pods []corev1.Pod, recs []autosizev1.Recommendation, skipMemory map[string]bool) (Result, error) {
	result := Result{ReasonCounts: map[string]int{}}
	if len(pods) == 0 || len(recs) == 0 {
		return result, nil
	}

	byName := map[string]autosizev1.Recommendation{}
	for i := range recs {
		byName[recs[i].ContainerName] = recs[i]
	}
	for i := range pods {
		current := &pods[i]
		desired := current.DeepCopy()
		patchPodSpec(&desired.Spec, byName, skipMemory)
		result.RestartPolicyWarnings += restartPolicyWarnings(current.Spec, desired.Spec)
		if !podSpecResourcesChanged(current.Spec, desired.Spec) {
			result.Noop++
			continue
		}
		if err := resizePod(ctx, c, desired); err != nil {
			result.Failed++
			for reason, count := range collectResizeReasonBuckets(ctx, c, *current) {
				result.ReasonCounts[reason] += count
			}
			continue
		}
		result.Resized++
	}
	if result.Failed > 0 {
		return result, fmt.Errorf("pod resize failed for %d pod(s); resized=%d noop=%d", result.Failed, result.Resized, result.Noop)
	}
	return result, nil
}

func restartPolicyWarnings(before, after corev1.PodSpec) int {
	if len(before.Containers) == 0 || len(before.Containers) != len(after.Containers) {
		return 0
	}
	total := 0
	for i := range before.Containers {
		bc := before.Containers[i]
		ac := after.Containers[i]
		if bc.Name != ac.Name {
			continue
		}
		cpuChanged := quantityChanged(bc.Resources.Requests, ac.Resources.Requests, corev1.ResourceCPU) ||
			quantityChanged(bc.Resources.Limits, ac.Resources.Limits, corev1.ResourceCPU)
		memChanged := quantityChanged(bc.Resources.Requests, ac.Resources.Requests, corev1.ResourceMemory) ||
			quantityChanged(bc.Resources.Limits, ac.Resources.Limits, corev1.ResourceMemory)
		if !cpuChanged && !memChanged {
			continue
		}
		for j := range bc.ResizePolicy {
			p := bc.ResizePolicy[j]
			if p.RestartPolicy != corev1.RestartContainer {
				continue
			}
			if (p.ResourceName == corev1.ResourceCPU && cpuChanged) || (p.ResourceName == corev1.ResourceMemory && memChanged) {
				total++
			}
		}
	}
	return total
}

func quantityChanged(before, after corev1.ResourceList, resourceName corev1.ResourceName) bool {
	if before == nil && after == nil {
		return false
	}
	bq, bok := before[resourceName]
	aq, aok := after[resourceName]
	if !bok && !aok {
		return false
	}
	if bok != aok {
		return true
	}
	return bq.Cmp(aq) != 0
}

func collectResizeReasonBuckets(ctx context.Context, c client.Client, fallback corev1.Pod) map[string]int {
	out := map[string]int{}
	pod := &corev1.Pod{}
	useFallback := false
	if err := getPod(ctx, c, client.ObjectKeyFromObject(&fallback), pod); err != nil {
		useFallback = true
	}
	reasons := extractResizeReasons(*pod)
	if useFallback || len(reasons) == 0 {
		reasons = extractResizeReasons(fallback)
	}
	if len(reasons) == 0 {
		out["unknown"] = 1
		return out
	}
	for _, reason := range reasons {
		out[bucketResizeReason(reason)]++
	}
	return out
}

func extractResizeReasons(pod corev1.Pod) []string {
	reasons := make([]string, 0, 2)
	anyResizeCond := false
	for i := range pod.Status.Conditions {
		cond := pod.Status.Conditions[i]
		if cond.Type != "PodResizePending" && cond.Type != "PodResizeInProgress" {
			continue
		}
		anyResizeCond = true
		if cond.Reason != "" {
			reasons = append(reasons, cond.Reason)
		}
	}
	if !anyResizeCond {
		for i := range pod.Status.Conditions {
			if pod.Status.Conditions[i].Reason != "" {
				reasons = append(reasons, pod.Status.Conditions[i].Reason)
			}
		}
	}
	sort.Strings(reasons)
	return reasons
}

func bucketResizeReason(reason string) string {
	r := strings.ToLower(reason)
	switch {
	case strings.Contains(r, "defer"):
		return "deferred"
	case strings.Contains(r, "infeasible"):
		return "infeasible"
	case strings.Contains(r, "forbidden"), strings.Contains(r, "denied"), strings.Contains(r, "unauthor"):
		return "forbidden"
	default:
		return "other"
	}
}

func podSpecResourcesChanged(before, after corev1.PodSpec) bool {
	if len(before.Containers) != len(after.Containers) {
		return true
	}
	for i := range before.Containers {
		if !reflect.DeepEqual(before.Containers[i].Resources, after.Containers[i].Resources) {
			return true
		}
	}
	return false
}

func patchPodSpec(spec *corev1.PodSpec, recs map[string]autosizev1.Recommendation, skipMemory map[string]bool) {
	for i := range spec.Containers {
		name := spec.Containers[i].Name
		r, ok := recs[name]
		if !ok {
			continue
		}
		if spec.Containers[i].Resources.Requests == nil {
			spec.Containers[i].Resources.Requests = corev1.ResourceList{}
		}
		if spec.Containers[i].Resources.Limits == nil {
			spec.Containers[i].Resources.Limits = corev1.ResourceList{}
		}
		spec.Containers[i].Resources.Requests[corev1.ResourceCPU] = r.CPURequest
		spec.Containers[i].Resources.Limits[corev1.ResourceCPU] = r.CPULimit
		if skipMemory != nil && skipMemory[name] {
			continue
		}
		spec.Containers[i].Resources.Requests[corev1.ResourceMemory] = r.MemoryRequest
		spec.Containers[i].Resources.Limits[corev1.ResourceMemory] = r.MemoryLimit
	}
}
