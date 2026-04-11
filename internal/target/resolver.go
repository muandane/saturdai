// Package target resolves WorkloadProfile targetRef to live workload objects.
package target

import (
	"context"
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ErrKindNotSupported is returned when targetRef.kind is not Deployment or StatefulSet.
var ErrKindNotSupported = errors.New("target kind not supported")

// Resolver resolves target references to API objects.
type Resolver struct {
	Client client.Client
}

// NewResolver returns a Resolver backed by the given client.
func NewResolver(c client.Client) *Resolver {
	return &Resolver{Client: c}
}

// Resolve returns the Deployment or StatefulSet for the profile namespace and targetRef.
func (r *Resolver) Resolve(ctx context.Context, namespace string, kind, name string) (runtime.Object, error) {
	switch kind {
	case "Deployment":
		obj := &appsv1.Deployment{}
		err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj)
		if err != nil {
			return nil, err
		}
		return obj, nil
	case "StatefulSet":
		obj := &appsv1.StatefulSet{}
		err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj)
		if err != nil {
			return nil, err
		}
		return obj, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrKindNotSupported, kind)
	}
}

// Selector returns a label selector for pods owned by the workload.
func Selector(obj runtime.Object) (labels.Selector, error) {
	switch t := obj.(type) {
	case *appsv1.Deployment:
		if t.Spec.Selector == nil {
			return nil, errors.New("deployment has nil selector")
		}
		return metav1.LabelSelectorAsSelector(t.Spec.Selector)
	case *appsv1.StatefulSet:
		if t.Spec.Selector == nil {
			return nil, errors.New("statefulset has nil selector")
		}
		return metav1.LabelSelectorAsSelector(t.Spec.Selector)
	default:
		return nil, fmt.Errorf("%w", ErrKindNotSupported)
	}
}

// TemplateContainerNames returns container names from the pod template (order preserved).
func TemplateContainerNames(obj runtime.Object) ([]string, error) {
	switch t := obj.(type) {
	case *appsv1.Deployment:
		return containerNamesFromPodSpec(&t.Spec.Template.Spec), nil
	case *appsv1.StatefulSet:
		return containerNamesFromPodSpec(&t.Spec.Template.Spec), nil
	default:
		return nil, fmt.Errorf("%w", ErrKindNotSupported)
	}
}

func containerNamesFromPodSpec(spec *corev1.PodSpec) []string {
	if spec == nil {
		return nil
	}
	out := make([]string, 0, len(spec.Containers))
	for i := range spec.Containers {
		out = append(out, spec.Containers[i].Name)
	}
	return out
}

// IsNotFound reports whether err is a not-found API error.
func IsNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}
