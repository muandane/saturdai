package target

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

// WorkloadKey uniquely identifies a workload within a namespace.
type WorkloadKey struct {
	Kind      string
	Namespace string
	Name      string
}

func (k WorkloadKey) String() string {
	return fmt.Sprintf("%s/%s/%s", k.Namespace, k.Kind, k.Name)
}

// DefaultMaxTargets is the cap when MaxTargets is zero or nil.
const DefaultMaxTargets = 50

// ListWorkloads returns workload keys in a namespace matching the selector.
func (r *Resolver) ListWorkloads(ctx context.Context, namespace string, sel autosizev1.WorkloadSelector, maxTargets int) ([]WorkloadKey, error) {
	if maxTargets <= 0 {
		maxTargets = DefaultMaxTargets
	}

	labelSel, err := selectorToLabels(sel)
	if err != nil {
		return nil, fmt.Errorf("parse label selector: %w", err)
	}

	includeDeployments := true
	includeStatefulSets := true
	if sel.Kinds != nil {
		if sel.Kinds.Deployment != nil {
			includeDeployments = *sel.Kinds.Deployment
		}
		if sel.Kinds.StatefulSet != nil {
			includeStatefulSets = *sel.Kinds.StatefulSet
		}
	}

	var keys []WorkloadKey
	opts := []client.ListOption{client.InNamespace(namespace)}
	if labelSel != nil {
		opts = append(opts, client.MatchingLabelsSelector{Selector: labelSel})
	}

	if includeDeployments {
		list := &appsv1.DeploymentList{}
		if err := r.Client.List(ctx, list, opts...); err != nil {
			return nil, fmt.Errorf("list deployments: %w", err)
		}
		for i := range list.Items {
			if len(keys) >= maxTargets {
				break
			}
			keys = append(keys, WorkloadKey{Kind: "Deployment", Namespace: namespace, Name: list.Items[i].Name})
		}
	}

	if includeStatefulSets && len(keys) < maxTargets {
		list := &appsv1.StatefulSetList{}
		if err := r.Client.List(ctx, list, opts...); err != nil {
			return nil, fmt.Errorf("list statefulsets: %w", err)
		}
		for i := range list.Items {
			if len(keys) >= maxTargets {
				break
			}
			keys = append(keys, WorkloadKey{Kind: "StatefulSet", Namespace: namespace, Name: list.Items[i].Name})
		}
	}

	return keys, nil
}

// ListClusterWorkloads returns workload keys across namespaces matching the namespace and workload selectors.
func (r *Resolver) ListClusterWorkloads(ctx context.Context, nsSel *metav1.LabelSelector, wSel autosizev1.WorkloadSelector, maxTargets int) ([]WorkloadKey, error) {
	if maxTargets <= 0 {
		maxTargets = 200
	}

	namespaces, err := r.matchNamespaces(ctx, nsSel)
	if err != nil {
		return nil, err
	}

	var keys []WorkloadKey
	for _, ns := range namespaces {
		remaining := maxTargets - len(keys)
		if remaining <= 0 {
			break
		}
		nsKeys, err := r.ListWorkloads(ctx, ns, wSel, remaining)
		if err != nil {
			return nil, fmt.Errorf("namespace %q: %w", ns, err)
		}
		keys = append(keys, nsKeys...)
	}
	return keys, nil
}

// MatchedNamespaceCount returns how many namespaces match the selector (for status).
func (r *Resolver) MatchedNamespaceCount(ctx context.Context, nsSel *metav1.LabelSelector) (int, error) {
	ns, err := r.matchNamespaces(ctx, nsSel)
	if err != nil {
		return 0, err
	}
	return len(ns), nil
}

func (r *Resolver) matchNamespaces(ctx context.Context, nsSel *metav1.LabelSelector) ([]string, error) {
	list := &corev1.NamespaceList{}
	var opts []client.ListOption
	if nsSel != nil {
		sel, err := metav1.LabelSelectorAsSelector(nsSel)
		if err != nil {
			return nil, fmt.Errorf("parse namespace selector: %w", err)
		}
		opts = append(opts, client.MatchingLabelsSelector{Selector: sel})
	}
	if err := r.Client.List(ctx, list, opts...); err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	names := make([]string, 0, len(list.Items))
	for i := range list.Items {
		names = append(names, list.Items[i].Name)
	}
	return names, nil
}

func selectorToLabels(sel autosizev1.WorkloadSelector) (labels.Selector, error) {
	if sel.LabelSelector == nil {
		if sel.SelectAll {
			return nil, nil
		}
		return nil, fmt.Errorf("labelSelector is required when selectAll is false")
	}
	return metav1.LabelSelectorAsSelector(sel.LabelSelector)
}
