// Package actuation applies recommended resources to running Pods in place.
package actuate

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

var resizePod = func(ctx context.Context, c client.Client, pod *corev1.Pod) error {
	return c.SubResource("resize").Update(ctx, pod)
}

// Result summarizes actuation results for a reconcile loop.
type Result struct {
	Resized int
	Noop    int
	Failed  int
}

// Apply updates running Pod container resources from recommendations using the pod/resize subresource.
func Apply(ctx context.Context, c client.Client, pods []corev1.Pod, recs []autosizev1.Recommendation, skipMemory map[string]bool) (Result, error) {
	result := Result{}
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
		if !podSpecResourcesChanged(current.Spec, desired.Spec) {
			result.Noop++
			continue
		}
		if err := resizePod(ctx, c, desired); err != nil {
			result.Failed++
			continue
		}
		result.Resized++
	}
	if result.Failed > 0 {
		return result, fmt.Errorf("pod resize failed for %d pod(s); resized=%d noop=%d", result.Failed, result.Resized, result.Noop)
	}
	return result, nil
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
