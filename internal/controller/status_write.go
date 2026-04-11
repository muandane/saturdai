package controller

import (
	"context"
	"fmt"
	"math"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

// benignStatusAPIErr returns true for errors that can happen when a WorkloadProfile
// is deleted or replaced while a status write is in flight (not a controller bug).
func benignStatusAPIErr(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsNotFound(err) || apierrors.IsGone(err) || apierrors.IsResourceExpired(err) {
		return true
	}
	// etcd/apiserver: UID precondition failed, object identity changed (common delete race).
	msg := err.Error()
	if strings.Contains(msg, "Precondition failed") && strings.Contains(msg, "UID") {
		return true
	}
	return false
}

func sanitizeStatusFloats(st *autosizev1.WorkloadProfileStatus) {
	if st == nil {
		return
	}
	for i := range st.Containers {
		c := &st.Containers[i]
		c.Stats.CPU.EMAShort = statusFiniteOrZero(c.Stats.CPU.EMAShort)
		c.Stats.CPU.EMALong = statusFiniteOrZero(c.Stats.CPU.EMALong)
		c.Stats.Memory.EMAShort = statusFiniteOrZero(c.Stats.Memory.EMAShort)
		c.Stats.Memory.EMALong = statusFiniteOrZero(c.Stats.Memory.EMALong)
	}
}

func statusFiniteOrZero(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return 0
	}
	return x
}

// persistStatus re-fetches the WorkloadProfile, copies profile.Status onto the live
// object, and writes status. Handles delete races and conflicts.
func (r *WorkloadProfileReconciler) persistStatus(ctx context.Context, profile *autosizev1.WorkloadProfile) error {
	logger := log.FromContext(ctx)
	sanitizeStatusFloats(&profile.Status)
	key := types.NamespacedName{Namespace: profile.Namespace, Name: profile.Name}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fresh := &autosizev1.WorkloadProfile{}
		if err := r.Client.Get(ctx, key, fresh); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("WorkloadProfile removed before status write; skipping", "WorkloadProfile", key)
				return nil
			}
			return fmt.Errorf("get WorkloadProfile for status: %w", err)
		}
		if !fresh.DeletionTimestamp.IsZero() {
			logger.V(1).Info("Skipping status write: WorkloadProfile is deleting", "WorkloadProfile", key)
			return nil
		}
		fresh.Status = profile.Status
		if err := r.Client.Status().Update(ctx, fresh); err != nil {
			if benignStatusAPIErr(err) {
				logger.Info("Status update skipped after benign API error", "WorkloadProfile", key, "error", err.Error())
				return nil
			}
			return err
		}
		return nil
	})
}
