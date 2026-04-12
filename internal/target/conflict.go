package target

import (
	"context"
	"fmt"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConflictResult holds the outcome of conflict detection for a set of workload keys.
type ConflictResult struct {
	// Free are workload keys not claimed by any other profile.
	Free []WorkloadKey
	// Conflicting maps workload key string to the name of the profile that already claims it.
	Conflicting map[string]string
}

// DetectConflicts checks which workload keys are already managed by existing WorkloadProfiles
// not owned by the given parent (parentKind/parentName). Returns free and conflicting sets.
func DetectConflicts(ctx context.Context, c client.Client, namespace string, keys []WorkloadKey, parentKind, parentName string) (*ConflictResult, error) {
	wpList := &autosizev1.WorkloadProfileList{}
	if err := c.List(ctx, wpList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("list workload profiles: %w", err)
	}

	claimed := map[string]string{}
	for i := range wpList.Items {
		wp := &wpList.Items[i]
		ownerKind := wp.Labels[autosizev1.LabelParentKind]
		ownerName := wp.Labels[autosizev1.LabelParentName]

		isOurs := ownerKind == parentKind && ownerName == parentName
		isManagedChild := wp.Labels[autosizev1.LabelManagedBy] != ""
		isStandalone := !isManagedChild

		if isOurs {
			continue
		}

		key := WorkloadKey{
			Kind:      wp.Spec.TargetRef.Kind,
			Namespace: namespace,
			Name:      wp.Spec.TargetRef.Name,
		}
		var owner string
		if isStandalone {
			owner = fmt.Sprintf("WorkloadProfile/%s", wp.Name)
		} else {
			owner = fmt.Sprintf("%s/%s", ownerKind, ownerName)
		}
		claimed[key.String()] = owner
	}

	result := &ConflictResult{Conflicting: map[string]string{}}
	for _, k := range keys {
		if owner, ok := claimed[k.String()]; ok {
			result.Conflicting[k.String()] = owner
		} else {
			result.Free = append(result.Free, k)
		}
	}
	return result, nil
}
