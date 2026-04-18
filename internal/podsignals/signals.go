// Package podsignals derives OOM and restart signals from pod status.
package podsignals

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Snapshot holds per-container signals for one reconcile.
type Snapshot struct {
	LastOOMKill  map[string]*metav1.Time
	RestartCount map[string]int32
}

// NewSnapshot builds an empty snapshot.
func NewSnapshot() *Snapshot {
	return &Snapshot{
		LastOOMKill:  map[string]*metav1.Time{},
		RestartCount: map[string]int32{},
	}
}

// MergePodStatus extracts restart counts and last OOM time from pod status.
func (s *Snapshot) MergePodStatus(pod *corev1.Pod) {
	if pod == nil {
		return
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.RestartCount > s.RestartCount[cs.Name] {
			s.RestartCount[cs.Name] = cs.RestartCount
		}
		if cs.LastTerminationState.Terminated == nil || cs.LastTerminationState.Terminated.Reason != "OOMKilled" {
			continue
		}
		finishedAt := cs.LastTerminationState.Terminated.FinishedAt
		if finishedAt.IsZero() {
			continue
		}
		prev := s.LastOOMKill[cs.Name]
		if prev == nil || finishedAt.After(prev.Time) {
			t := finishedAt.DeepCopy()
			s.LastOOMKill[cs.Name] = t
		}
	}
}
