package controller

import (
	"time"

	autosizev1 "github.com/muandane/saturdai/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const oomProtectionWindow = 10 * time.Minute

// applyLastOOMKillFromSnapshot sets st.Stats.LastOOMKill from the pod-signal snapshot.
// A nil time clears the field so status reflects current pod observation only.
func applyLastOOMKillFromSnapshot(st *autosizev1.ProfileContainerStatus, t *metav1.Time) {
	if st == nil {
		return
	}
	if t == nil {
		st.Stats.LastOOMKill = nil
		return
	}
	st.Stats.LastOOMKill = t.DeepCopy()
}

// persistedLastOOMKill merges observed OOM time into persistent state and returns an unexpired value.
func persistedLastOOMKill(
	now time.Time,
	container string,
	observed *metav1.Time,
	store map[string]*metav1.Time,
) *metav1.Time {
	if store == nil {
		if observed == nil {
			return nil
		}
		return observed.DeepCopy()
	}
	if observed != nil {
		store[container] = observed.DeepCopy()
	}
	t := store[container]
	if t == nil {
		delete(store, container)
		return nil
	}
	if now.Sub(t.Time) >= oomProtectionWindow {
		delete(store, container)
		return nil
	}
	return t.DeepCopy()
}
