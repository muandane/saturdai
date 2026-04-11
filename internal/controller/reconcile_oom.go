package controller

import (
	autosizev1 "github.com/muandane/saturdai/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
