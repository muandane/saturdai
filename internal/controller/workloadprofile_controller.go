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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	"github.com/muandane/saturdai/internal/changepoint"
	"github.com/muandane/saturdai/internal/kubelet"
	"github.com/muandane/saturdai/internal/mlstate"
	"github.com/muandane/saturdai/internal/target"
)

// WorkloadProfileReconciler reconciles a WorkloadProfile object.
type WorkloadProfileReconciler struct {
	Client  client.Client
	Scheme  *runtime.Scheme
	Target  *target.Resolver
	Kubelet kubelet.Interface
	// Clock returns the current time (inject for tests; nil defaults to time.Now in reconcile).
	Clock func() time.Time
	// MLState persists CUSUM, feedback, and HW state (nil skips persistence; in-memory only).
	MLState mlstate.Repository
	// Detector notifies handlers on CUSUM shift (optional).
	Detector *changepoint.Detector
}

// +kubebuilder:rbac:groups=autosize.saturdai.auto,resources=workloadprofiles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autosize.saturdai.auto,resources=workloadprofiles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autosize.saturdai.auto,resources=workloadprofiles/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes/proxy,verbs=get
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile implements the autosizing observe/actuation loop.
func (r *WorkloadProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	profile := &autosizev1.WorkloadProfile{}
	if err := r.Client.Get(ctx, req.NamespacedName, profile); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !profile.DeletionTimestamp.IsZero() {
		logger.V(1).Info("Skipping reconcile: WorkloadProfile is deleting")
		return ctrl.Result{}, nil
	}
	customRequeue, err := r.reconcile(ctx, profile)
	if err != nil {
		logger.Error(err, "reconcile WorkloadProfile")
		// Do not set RequeueAfter when err != nil; controller-runtime ignores it.
		return ctrl.Result{}, err
	}
	if customRequeue != nil {
		return ctrl.Result{RequeueAfter: *customRequeue}, nil
	}
	return ctrl.Result{RequeueAfter: requeueAfter(profile)}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autosizev1.WorkloadProfile{}).
		Named("workloadprofile").
		Complete(r)
}
