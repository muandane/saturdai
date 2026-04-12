package controller

import (
	"context"
	"crypto/sha256"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/target"
)

// childName returns a deterministic name for a child WorkloadProfile.
func childName(parentName string, key target.WorkloadKey) string {
	h := sha256.Sum256(fmt.Appendf(nil, "%s/%s/%s/%s", parentName, key.Namespace, key.Kind, key.Name))
	return fmt.Sprintf("%s-%x", truncate(parentName, 40), h[:4])
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// syncChildren creates/updates child WorkloadProfiles for free keys and prunes stale children.
// Returns the list of children that were synced.
func syncChildren(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	parent client.Object,
	parentKind string,
	policy autosizev1.PolicySpec,
	freeKeys []target.WorkloadKey,
) ([]autosizev1.ChildReference, error) {
	logger := log.FromContext(ctx)

	desiredNames := map[string]struct{}{}
	var children []autosizev1.ChildReference

	for _, key := range freeKeys {
		name := childName(parent.GetName(), key)
		desiredNames[name] = struct{}{}

		child := &autosizev1.WorkloadProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: key.Namespace,
			},
		}

		result, err := controllerutil.CreateOrUpdate(ctx, c, child, func() error {
			child.Labels = map[string]string{
				autosizev1.LabelManagedBy:  "true",
				autosizev1.LabelParentKind: parentKind,
				autosizev1.LabelParentName: parent.GetName(),
			}
			child.Spec = autosizev1.WorkloadProfileSpec{
				TargetRef: autosizev1.WorkloadTargetRef{
					Kind: key.Kind,
					Name: key.Name,
				},
				Mode:                      policy.Mode,
				Containers:                policy.Containers,
				CooldownMinutes:           policy.CooldownMinutes,
				CollectionIntervalSeconds: policy.CollectionIntervalSeconds,
			}
			return controllerutil.SetOwnerReference(parent, child, scheme)
		})
		if err != nil {
			return nil, fmt.Errorf("sync child %s: %w", name, err)
		}
		if result != controllerutil.OperationResultNone {
			logger.V(1).Info("synced child WorkloadProfile", "child", name, "operation", result, "target", key.String())
		}

		children = append(children, autosizev1.ChildReference{
			Name:       name,
			TargetKind: key.Kind,
			TargetName: key.Name,
		})
	}

	// Prune stale children owned by this parent.
	wpList := &autosizev1.WorkloadProfileList{}
	if err := c.List(ctx, wpList,
		client.InNamespace(parent.GetNamespace()),
		client.MatchingLabels{
			autosizev1.LabelManagedBy:  "true",
			autosizev1.LabelParentKind: parentKind,
			autosizev1.LabelParentName: parent.GetName(),
		},
	); err != nil {
		return nil, fmt.Errorf("list children for prune: %w", err)
	}
	for i := range wpList.Items {
		if _, keep := desiredNames[wpList.Items[i].Name]; !keep {
			logger.V(1).Info("pruning stale child WorkloadProfile", "child", wpList.Items[i].Name)
			if err := c.Delete(ctx, &wpList.Items[i]); err != nil {
				return nil, fmt.Errorf("delete stale child %s: %w", wpList.Items[i].Name, err)
			}
		}
	}

	return children, nil
}

// syncClusterChildren is like syncChildren but for cluster-scoped parents that create children across namespaces.
func syncClusterChildren(
	ctx context.Context,
	c client.Client,
	parent client.Object,
	parentKind string,
	policy autosizev1.PolicySpec,
	freeKeys []target.WorkloadKey,
) ([]autosizev1.ClusterChildReference, error) {
	logger := log.FromContext(ctx)

	type nsName struct{ ns, name string }
	desiredNames := map[nsName]struct{}{}
	var children []autosizev1.ClusterChildReference

	for _, key := range freeKeys {
		name := childName(parent.GetName(), key)
		desiredNames[nsName{key.Namespace, name}] = struct{}{}

		child := &autosizev1.WorkloadProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: key.Namespace,
			},
		}

		result, err := controllerutil.CreateOrUpdate(ctx, c, child, func() error {
			child.Labels = map[string]string{
				autosizev1.LabelManagedBy:  "true",
				autosizev1.LabelParentKind: parentKind,
				autosizev1.LabelParentName: parent.GetName(),
			}
			child.Spec = autosizev1.WorkloadProfileSpec{
				TargetRef: autosizev1.WorkloadTargetRef{
					Kind: key.Kind,
					Name: key.Name,
				},
				Mode:                      policy.Mode,
				Containers:                policy.Containers,
				CooldownMinutes:           policy.CooldownMinutes,
				CollectionIntervalSeconds: policy.CollectionIntervalSeconds,
			}
			// Cluster-scoped parent cannot set cross-namespace ownerReference;
			// rely on labels for GC and pruning.
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("sync child %s/%s: %w", key.Namespace, name, err)
		}
		if result != controllerutil.OperationResultNone {
			logger.V(1).Info("synced child WorkloadProfile", "child", name, "namespace", key.Namespace, "operation", result)
		}

		children = append(children, autosizev1.ClusterChildReference{
			Namespace:  key.Namespace,
			Name:       name,
			TargetKind: key.Kind,
			TargetName: key.Name,
		})
	}

	// Prune stale children across all namespaces.
	wpList := &autosizev1.WorkloadProfileList{}
	if err := c.List(ctx, wpList,
		client.MatchingLabels{
			autosizev1.LabelManagedBy:  "true",
			autosizev1.LabelParentKind: parentKind,
			autosizev1.LabelParentName: parent.GetName(),
		},
	); err != nil {
		return nil, fmt.Errorf("list children for prune: %w", err)
	}
	for i := range wpList.Items {
		wp := &wpList.Items[i]
		if _, keep := desiredNames[nsName{wp.Namespace, wp.Name}]; !keep {
			logger.V(1).Info("pruning stale child WorkloadProfile", "child", wp.Name, "namespace", wp.Namespace)
			if err := c.Delete(ctx, wp); client.IgnoreNotFound(err) != nil {
				return nil, fmt.Errorf("delete stale child %s/%s: %w", wp.Namespace, wp.Name, err)
			}
		}
	}

	return children, nil
}
