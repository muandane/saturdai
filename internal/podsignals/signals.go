// Package podsignals derives OOM, restart, and throttle signals from pods and kubelet stats.
package podsignals

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Snapshot holds per-container signals for one reconcile.
type Snapshot struct {
	LastOOMKill    map[string]*metav1.Time
	RestartCount   map[string]int32
	ThrottleRatios map[string]float64
}

// NewSnapshot builds an empty snapshot.
func NewSnapshot() *Snapshot {
	return &Snapshot{
		LastOOMKill:    map[string]*metav1.Time{},
		RestartCount:   map[string]int32{},
		ThrottleRatios: map[string]float64{},
	}
}

// MergePodStatus extracts restart counts and last OOM time from pod status.
func (s *Snapshot) MergePodStatus(pod *corev1.Pod) {
	if pod == nil {
		return
	}
	for _, cs := range pod.Status.ContainerStatuses {
		s.RestartCount[cs.Name] = cs.RestartCount
		if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
			t := cs.LastTerminationState.Terminated.FinishedAt
			s.LastOOMKill[cs.Name] = &t
		}
	}
}

// SetThrottleRatio stores throttledUsage/usage for a container (0 if unknown).
func (s *Snapshot) SetThrottleRatio(container string, throttledNano, usageNano uint64) {
	if usageNano == 0 {
		return
	}
	s.ThrottleRatios[container] = float64(throttledNano) / float64(usageNano)
}
