// Package kubelet reads kubelet /stats/summary via the apiserver node proxy.
package kubelet

import (
	"encoding/json"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Summary is a subset of kubelet stats/summary JSON.
type Summary struct {
	Pods []PodStats `json:"pods"`
}

// PodStats holds per-pod stats from summary.
type PodStats struct {
	PodRef     PodReference     `json:"podRef"`
	Containers []ContainerStats `json:"containers"`
	CPU        *CPUStats        `json:"cpu,omitempty"`
	Memory     *MemoryStats     `json:"memory,omitempty"`
}

// PodReference identifies a pod.
type PodReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	UID       string `json:"uid"`
}

// ContainerStats holds per-container resource usage in summary.
type ContainerStats struct {
	Name   string       `json:"name"`
	CPU    *CPUStats    `json:"cpu,omitempty"`
	Memory *MemoryStats `json:"memory,omitempty"`
	Rootfs *FsStats     `json:"rootfs,omitempty"`
}

// CPUStats mirrors kubelet CPU stats.
type CPUStats struct {
	Time metav1.Time `json:"time"`
	// UsageNanoCores is instantaneous usage.
	UsageNanoCores *uint64 `json:"usageNanoCores,omitempty"`
	// ThrottledUsageNanoCores may be set when throttling data exists.
	ThrottledUsageNanoCores *uint64 `json:"throttledUsageNanoCores,omitempty"`
}

// MemoryStats mirrors kubelet memory stats.
type MemoryStats struct {
	Time            metav1.Time `json:"time"`
	UsageBytes      *uint64     `json:"usageBytes,omitempty"`
	WorkingSetBytes *uint64     `json:"workingSetBytes,omitempty"`
}

// FsStats is a minimal placeholder for optional fields.
type FsStats struct {
	UsedBytes *uint64 `json:"usedBytes,omitempty"`
}

// ParseSummary decodes JSON from stats/summary.
func ParseSummary(data []byte) (*Summary, error) {
	var s Summary
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// DefaultFetchTimeout bounds kubelet proxy calls.
const DefaultFetchTimeout = 15 * time.Second
