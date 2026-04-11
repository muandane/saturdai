// Package actuation applies recommended resources to workload pod templates.
package actuate

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

// Apply updates Deployment or StatefulSet pod template resources from recommendations.
func Apply(ctx context.Context, c client.Client, obj runtime.Object, recs []autosizev1.Recommendation, skipMemory map[string]bool) error {
	byName := map[string]autosizev1.Recommendation{}
	for i := range recs {
		byName[recs[i].ContainerName] = recs[i]
	}
	switch t := obj.(type) {
	case *appsv1.Deployment:
		cp := t.DeepCopy()
		if err := patchPodSpec(&cp.Spec.Template.Spec, byName, skipMemory); err != nil {
			return err
		}
		return c.Update(ctx, cp)
	case *appsv1.StatefulSet:
		cp := t.DeepCopy()
		if err := patchPodSpec(&cp.Spec.Template.Spec, byName, skipMemory); err != nil {
			return err
		}
		return c.Update(ctx, cp)
	default:
		return fmt.Errorf("unsupported target type %T", obj)
	}
}

func patchPodSpec(spec *corev1.PodSpec, recs map[string]autosizev1.Recommendation, skipMemory map[string]bool) error {
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
	return nil
}
