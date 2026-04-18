/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/target"
)

const namespaceProfileResyncInterval = 60 * time.Second

// NamespaceProfileReconciler reconciles a NamespaceProfile object.
type NamespaceProfileReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Target *target.Resolver
}

// +kubebuilder:rbac:groups=autosize.saturdai.auto,resources=namespaceprofiles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autosize.saturdai.auto,resources=namespaceprofiles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autosize.saturdai.auto,resources=namespaceprofiles/finalizers,verbs=update
// +kubebuilder:rbac:groups=autosize.saturdai.auto,resources=workloadprofiles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch

func (r *NamespaceProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	profile := &autosizev1.NamespaceProfile{}
	if err := r.Get(ctx, req.NamespacedName, profile); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !profile.DeletionTimestamp.IsZero() {
		logger.V(1).Info("skipping reconcile: NamespaceProfile is deleting")
		return ctrl.Result{}, nil
	}

	maxTargets := int(effectiveMaxTargets(profile.Spec.MaxTargets, target.DefaultMaxTargets))

	keys, err := r.Target.ListWorkloads(ctx, profile.Namespace, profile.Spec.WorkloadSelector, maxTargets)
	if err != nil {
		setNSPCondition(profile, autosizev1.ConditionTypeSelectorResolved, metav1.ConditionFalse, "ListError", err.Error())
		if uerr := r.patchNSPStatus(ctx, profile); uerr != nil {
			return ctrl.Result{}, uerr
		}
		return ctrl.Result{}, err
	}

	profile.Status.ResolvedCount = int32(len(keys))

	if len(keys) == 0 {
		setNSPCondition(profile, autosizev1.ConditionTypeSelectorResolved, metav1.ConditionFalse, "NoTargetsFound", "selector matched zero workloads")
		setNSPCondition(profile, autosizev1.ConditionTypeSelectorConflict, metav1.ConditionFalse, "NoConflict", "no targets to conflict")
		setNSPCondition(profile, autosizev1.ConditionTypeChildrenSynced, metav1.ConditionFalse, "NoTargets", "nothing to sync")
		profile.Status.ActiveChildren = 0
		profile.Status.Children = nil
		if uerr := r.patchNSPStatus(ctx, profile); uerr != nil {
			return ctrl.Result{}, uerr
		}
		return ctrl.Result{RequeueAfter: namespaceProfileResyncInterval}, nil
	}

	setNSPCondition(profile, autosizev1.ConditionTypeSelectorResolved, metav1.ConditionTrue, "Resolved",
		fmt.Sprintf("matched %d workload(s)", len(keys)))

	conflicts, err := target.DetectConflicts(ctx, r.Client, profile.Namespace, keys, "NamespaceProfile", profile.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("detect conflicts: %w", err)
	}

	if len(conflicts.Conflicting) > 0 {
		var msgs []string
		for k, owner := range conflicts.Conflicting {
			msgs = append(msgs, fmt.Sprintf("%s claimed by %s", k, owner))
		}
		setNSPCondition(profile, autosizev1.ConditionTypeSelectorConflict, metav1.ConditionTrue, "OverlappingSelector",
			strings.Join(msgs, "; "))
	} else {
		setNSPCondition(profile, autosizev1.ConditionTypeSelectorConflict, metav1.ConditionFalse, "NoConflict", "no overlapping selectors")
	}

	children, err := syncChildren(ctx, r.Client, r.Scheme, profile, "NamespaceProfile", profile.Spec.Policy, conflicts.Free)
	if err != nil {
		setNSPCondition(profile, autosizev1.ConditionTypeChildrenSynced, metav1.ConditionFalse, "SyncError", err.Error())
		if uerr := r.patchNSPStatus(ctx, profile); uerr != nil {
			return ctrl.Result{}, uerr
		}
		return ctrl.Result{}, err
	}

	profile.Status.ActiveChildren = int32(len(children))
	profile.Status.Children = children
	setNSPCondition(profile, autosizev1.ConditionTypeChildrenSynced, metav1.ConditionTrue, "Synced",
		fmt.Sprintf("%d child WorkloadProfile(s) synced", len(children)))

	if uerr := r.patchNSPStatus(ctx, profile); uerr != nil {
		return ctrl.Result{}, uerr
	}

	return ctrl.Result{RequeueAfter: namespaceProfileResyncInterval}, nil
}

func (r *NamespaceProfileReconciler) patchNSPStatus(ctx context.Context, profile *autosizev1.NamespaceProfile) error {
	key := client.ObjectKeyFromObject(profile)
	status := profile.Status
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fresh := &autosizev1.NamespaceProfile{}
		if err := r.Get(ctx, key, fresh); err != nil {
			return err
		}
		fresh.Status = status
		return r.Status().Update(ctx, fresh)
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autosizev1.NamespaceProfile{}).
		Owns(&autosizev1.WorkloadProfile{}).
		Watches(&appsv1.Deployment{}, handler.EnqueueRequestsFromMapFunc(r.mapWorkloadToNSP)).
		Watches(&appsv1.StatefulSet{}, handler.EnqueueRequestsFromMapFunc(r.mapWorkloadToNSP)).
		Named("namespaceprofile").
		Complete(r)
}

// mapWorkloadToNSP enqueues all NamespaceProfiles in the same namespace when a Deployment/StatefulSet changes.
func (r *NamespaceProfileReconciler) mapWorkloadToNSP(ctx context.Context, obj client.Object) []ctrl.Request {
	list := &autosizev1.NamespaceProfileList{}
	if err := r.List(ctx, list, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}
	reqs := make([]ctrl.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, ctrl.Request{
			NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
		})
	}
	return reqs
}

func setNSPCondition(profile *autosizev1.NamespaceProfile, typ string, status metav1.ConditionStatus, reason, message string) {
	apimeta.SetStatusCondition(&profile.Status.Conditions, metav1.Condition{
		Type:               typ,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: profile.Generation,
	})
}

func effectiveMaxTargets(specMax *int32, defaultMax int) int32 {
	if specMax != nil && *specMax > 0 {
		return *specMax
	}
	return int32(defaultMax)
}
