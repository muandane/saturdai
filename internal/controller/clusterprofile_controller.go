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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/target"
)

const (
	clusterProfileResyncInterval    = 90 * time.Second
	clusterProfileDefaultMaxTargets = 200
)

// ClusterProfileReconciler reconciles a ClusterProfile object.
type ClusterProfileReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Target *target.Resolver
}

// +kubebuilder:rbac:groups=autosize.saturdai.auto,resources=clusterprofiles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autosize.saturdai.auto,resources=clusterprofiles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autosize.saturdai.auto,resources=clusterprofiles/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch

func (r *ClusterProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	profile := &autosizev1.ClusterProfile{}
	if err := r.Get(ctx, req.NamespacedName, profile); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !profile.DeletionTimestamp.IsZero() {
		logger.V(1).Info("skipping reconcile: ClusterProfile is deleting")
		return ctrl.Result{}, nil
	}

	maxTargets := int(effectiveMaxTargets(profile.Spec.MaxTargets, clusterProfileDefaultMaxTargets))

	nsCount, err := r.Target.MatchedNamespaceCount(ctx, profile.Spec.NamespaceSelector)
	if err != nil {
		setCSPCondition(profile, autosizev1.ConditionTypeSelectorResolved, metav1.ConditionFalse, "NamespaceListError", err.Error())
		if uerr := r.patchCSPStatus(ctx, profile); uerr != nil {
			return ctrl.Result{}, uerr
		}
		return ctrl.Result{}, err
	}
	profile.Status.MatchedNamespaces = int32(nsCount)

	keys, err := r.Target.ListClusterWorkloads(ctx, profile.Spec.NamespaceSelector, profile.Spec.WorkloadSelector, maxTargets)
	if err != nil {
		setCSPCondition(profile, autosizev1.ConditionTypeSelectorResolved, metav1.ConditionFalse, "ListError", err.Error())
		if uerr := r.patchCSPStatus(ctx, profile); uerr != nil {
			return ctrl.Result{}, uerr
		}
		return ctrl.Result{}, err
	}

	profile.Status.ResolvedCount = int32(len(keys))

	if len(keys) == 0 {
		setCSPCondition(profile, autosizev1.ConditionTypeSelectorResolved, metav1.ConditionFalse, "NoTargetsFound", "selector matched zero workloads across all namespaces")
		setCSPCondition(profile, autosizev1.ConditionTypeSelectorConflict, metav1.ConditionFalse, "NoConflict", "no targets to conflict")
		setCSPCondition(profile, autosizev1.ConditionTypeChildrenSynced, metav1.ConditionFalse, "NoTargets", "nothing to sync")
		profile.Status.ActiveChildren = 0
		profile.Status.Children = nil
		if uerr := r.patchCSPStatus(ctx, profile); uerr != nil {
			return ctrl.Result{}, uerr
		}
		return ctrl.Result{RequeueAfter: clusterProfileResyncInterval}, nil
	}

	setCSPCondition(profile, autosizev1.ConditionTypeSelectorResolved, metav1.ConditionTrue, "Resolved",
		fmt.Sprintf("matched %d workload(s) across %d namespace(s)", len(keys), nsCount))

	// Conflict detection per namespace.
	var allFree []target.WorkloadKey
	var conflictMsgs []string
	byNS := groupKeysByNamespace(keys)
	for ns, nsKeys := range byNS {
		conflicts, err := target.DetectConflicts(ctx, r.Client, ns, nsKeys, "ClusterProfile", profile.Name)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("detect conflicts in %s: %w", ns, err)
		}
		allFree = append(allFree, conflicts.Free...)
		for k, owner := range conflicts.Conflicting {
			conflictMsgs = append(conflictMsgs, fmt.Sprintf("%s claimed by %s", k, owner))
		}
	}

	if len(conflictMsgs) > 0 {
		setCSPCondition(profile, autosizev1.ConditionTypeSelectorConflict, metav1.ConditionTrue, "OverlappingSelector",
			strings.Join(conflictMsgs, "; "))
	} else {
		setCSPCondition(profile, autosizev1.ConditionTypeSelectorConflict, metav1.ConditionFalse, "NoConflict", "no overlapping selectors")
	}

	children, err := syncClusterChildren(ctx, r.Client, profile, "ClusterProfile", profile.Spec.Policy, allFree)
	if err != nil {
		setCSPCondition(profile, autosizev1.ConditionTypeChildrenSynced, metav1.ConditionFalse, "SyncError", err.Error())
		if uerr := r.patchCSPStatus(ctx, profile); uerr != nil {
			return ctrl.Result{}, uerr
		}
		return ctrl.Result{}, err
	}

	profile.Status.ActiveChildren = int32(len(children))
	profile.Status.Children = children
	setCSPCondition(profile, autosizev1.ConditionTypeChildrenSynced, metav1.ConditionTrue, "Synced",
		fmt.Sprintf("%d child WorkloadProfile(s) synced", len(children)))

	if uerr := r.patchCSPStatus(ctx, profile); uerr != nil {
		return ctrl.Result{}, uerr
	}

	return ctrl.Result{RequeueAfter: clusterProfileResyncInterval}, nil
}

func (r *ClusterProfileReconciler) patchCSPStatus(ctx context.Context, profile *autosizev1.ClusterProfile) error {
	return r.Status().Update(ctx, profile)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autosizev1.ClusterProfile{}).
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(r.mapNamespaceToCSP)).
		Watches(&appsv1.Deployment{}, handler.EnqueueRequestsFromMapFunc(r.mapWorkloadToCSP)).
		Watches(&appsv1.StatefulSet{}, handler.EnqueueRequestsFromMapFunc(r.mapWorkloadToCSP)).
		Named("clusterprofile").
		Complete(r)
}

func (r *ClusterProfileReconciler) mapNamespaceToCSP(ctx context.Context, _ client.Object) []ctrl.Request {
	return r.allClusterProfiles(ctx)
}

func (r *ClusterProfileReconciler) mapWorkloadToCSP(ctx context.Context, _ client.Object) []ctrl.Request {
	return r.allClusterProfiles(ctx)
}

func (r *ClusterProfileReconciler) allClusterProfiles(ctx context.Context) []ctrl.Request {
	list := &autosizev1.ClusterProfileList{}
	if err := r.List(ctx, list); err != nil {
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

func setCSPCondition(profile *autosizev1.ClusterProfile, typ string, status metav1.ConditionStatus, reason, message string) {
	c := metav1.Condition{
		Type:               typ,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: profile.Generation,
		LastTransitionTime: metav1.Now(),
	}
	for i := range profile.Status.Conditions {
		if profile.Status.Conditions[i].Type == typ {
			profile.Status.Conditions[i] = c
			return
		}
	}
	profile.Status.Conditions = append(profile.Status.Conditions, c)
}

func groupKeysByNamespace(keys []target.WorkloadKey) map[string][]target.WorkloadKey {
	m := make(map[string][]target.WorkloadKey)
	for _, k := range keys {
		m[k.Namespace] = append(m[k.Namespace], k)
	}
	return m
}
