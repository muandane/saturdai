package dashboard

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

func fetchWorkloadResources(ctx context.Context, c client.Client, profile *autosizev1.WorkloadProfile, containerNames []string) (map[string]corev1.ResourceRequirements, error) {
	if profile == nil {
		return nil, nil
	}
	ref := profile.Spec.TargetRef
	ns := profile.Namespace
	key := types.NamespacedName{Namespace: ns, Name: ref.Name}

	switch ref.Kind {
	case "Deployment":
		var d appsv1.Deployment
		if err := c.Get(ctx, key, &d); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
		return resourcesForNames(&d.Spec.Template.Spec, containerNames), nil
	case "StatefulSet":
		var s appsv1.StatefulSet
		if err := c.Get(ctx, key, &s); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
		return resourcesForNames(&s.Spec.Template.Spec, containerNames), nil
	default:
		return nil, fmt.Errorf("unsupported target kind %q", ref.Kind)
	}
}

func resourcesForNames(spec *corev1.PodSpec, names []string) map[string]corev1.ResourceRequirements {
	out := make(map[string]corev1.ResourceRequirements)
	want := make(map[string]struct{}, len(names))
	for _, n := range names {
		want[n] = struct{}{}
	}
	for i := range spec.Containers {
		ct := spec.Containers[i]
		if _, ok := want[ct.Name]; ok {
			out[ct.Name] = ct.Resources
		}
	}
	return out
}
