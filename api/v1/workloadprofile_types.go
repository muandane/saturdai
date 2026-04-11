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

package v1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadTargetRef identifies a Deployment or StatefulSet in the same namespace as the profile.
type WorkloadTargetRef struct {
	// Kind is the workload type to resize.
	// +kubebuilder:validation:Enum=Deployment;StatefulSet
	Kind string `json:"kind"`

	// Name is the metadata.name of the target workload.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ContainerOverride sets optional min/max resource bounds for a container name from the pod template.
type ContainerOverride struct {
	// Name matches a container name in the workload pod template.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// +optional
	MinCPU *resource.Quantity `json:"minCPU,omitempty"`

	// +optional
	MaxCPU *resource.Quantity `json:"maxCPU,omitempty"`

	// +optional
	MinMemory *resource.Quantity `json:"minMemory,omitempty"`

	// +optional
	MaxMemory *resource.Quantity `json:"maxMemory,omitempty"`
}

// WorkloadProfileSpec defines the desired state of WorkloadProfile.
type WorkloadProfileSpec struct {
	// TargetRef selects the Deployment or StatefulSet to manage.
	TargetRef WorkloadTargetRef `json:"targetRef"`

	// Mode selects recommendation strategy: cost, balanced, resilience, or burst.
	// +kubebuilder:validation:Enum=cost;balanced;resilience;burst
	// +kubebuilder:default=balanced
	// +optional
	Mode string `json:"mode,omitempty"`

	// Containers sets optional per-container min/max resource bounds.
	// +kubebuilder:validation:MaxItems=20
	// +optional
	Containers []ContainerOverride `json:"containers,omitempty"`

	// CooldownMinutes is the minimum interval between actuation patches. Default 15.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=15
	// +optional
	CooldownMinutes *int32 `json:"cooldownMinutes,omitempty"`

	// CollectionIntervalSeconds is how often metrics are collected. Default 30. Range 10–300.
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=300
	// +kubebuilder:default=30
	// +optional
	CollectionIntervalSeconds *int32 `json:"collectionIntervalSeconds,omitempty"`
}

// CPUStats holds aggregate CPU metrics for status.
type CPUStats struct {
	EMAShort float64 `json:"emaShort"`
	EMALong  float64 `json:"emaLong"`
	// Sketch is a base64-encoded DDSketch protobuf.
	Sketch string `json:"sketch"`
	// QuadrantSketches holds base64 DDSketches for UTC 6h buckets (index 0=00–06, 1=06–12, 2=12–18, 3=18–24).
	// +kubebuilder:validation:MaxItems=4
	// +optional
	QuadrantSketches []string `json:"quadrantSketches,omitempty"`
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
}

// MemoryStats holds aggregate memory metrics for status.
type MemoryStats struct {
	EMAShort float64 `json:"emaShort"`
	EMALong  float64 `json:"emaLong"`
	Sketch   string  `json:"sketch"`
	// QuadrantSketches holds base64 DDSketches for UTC 6h buckets.
	// +kubebuilder:validation:MaxItems=4
	// +optional
	QuadrantSketches []string `json:"quadrantSketches,omitempty"`
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
	// SlopeStreak counts consecutive reconciles where memory EMAShort strictly increased vs the prior
	// persisted reconcile (spec §6). Persisted for controller restart safety. Resets when not increasing.
	// +optional
	SlopeStreak int32 `json:"slopeStreak,omitempty"`
	// SlopePositive is true when SlopeStreak >= the controller threshold (default 5 cycles); blocks memory downsize (070).
	SlopePositive bool `json:"slopePositive"`
}

// ContainerResourceStats is observed stats for one logical container (pod template name).
type ContainerResourceStats struct {
	CPU    CPUStats    `json:"cpu"`
	Memory MemoryStats `json:"memory"`
	// +optional
	LastOOMKill  *metav1.Time `json:"lastOOMKill,omitempty"`
	RestartCount int32        `json:"restartCount"`
}

// ProfileContainerStatus binds a container name to its stats in status.
type ProfileContainerStatus struct {
	Name  string                 `json:"name"`
	Stats ContainerResourceStats `json:"stats"`
}

// Recommendation is a deterministic suggested patch for one container.
type Recommendation struct {
	ContainerName string `json:"containerName"`

	CPURequest    resource.Quantity `json:"cpuRequest"`
	CPULimit      resource.Quantity `json:"cpuLimit"`
	MemoryRequest resource.Quantity `json:"memoryRequest"`
	MemoryLimit   resource.Quantity `json:"memoryLimit"`

	Rationale string `json:"rationale"`
}

// WorkloadProfileStatus defines the observed state of WorkloadProfile.
type WorkloadProfileStatus struct {
	// Containers holds per-container aggregates and signals. Bounded for etcd size (max 20).
	// +kubebuilder:validation:MaxItems=20
	// +optional
	Containers []ProfileContainerStatus `json:"containers,omitempty"`

	// MetricsRecommendations are requests/limits from the recommendation engine before safety.Apply
	// (cooldown, OOM/throttle overrides, 70% decrease floor, trend guard). Omitted when empty.
	// +kubebuilder:validation:MaxItems=20
	// +optional
	MetricsRecommendations []Recommendation `json:"metricsRecommendations,omitempty"`

	// Recommendations are requests/limits after safety.Apply, with human-readable rationale.
	// CPU (and memory when actuation is not skipping memory) match what actuation and the pod webhook apply when enabled.
	// Under trend guard (slopePositive), memory may still reflect uncapped engine intent while the live template stays frozen;
	// rationale includes "trend_guard" and actuation omits memory PATCH for that container (see spec §9).
	// +kubebuilder:validation:MaxItems=20
	// +optional
	Recommendations []Recommendation `json:"recommendations,omitempty"`

	// +optional
	LastApplied *metav1.Time `json:"lastApplied,omitempty"`

	// +optional
	LastEvaluated *metav1.Time `json:"lastEvaluated,omitempty"`

	// DownsizePauseCyclesRemaining counts reconcile cycles where downsizing is blocked after a restart spike (delta > 3).
	// +optional
	DownsizePauseCyclesRemaining int32 `json:"downsizePauseCyclesRemaining,omitempty"`

	// conditions represent the current state of the WorkloadProfile resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=workloadprofiles,shortName=wp
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.targetRef.name`
// +kubebuilder:printcolumn:name="Pause",type=integer,JSONPath=`.status.downsizePauseCyclesRemaining`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// WorkloadProfile is the Schema for the workloadprofiles API.
type WorkloadProfile struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec WorkloadProfileSpec `json:"spec"`

	// +optional
	Status WorkloadProfileStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WorkloadProfileList contains a list of WorkloadProfile.
type WorkloadProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WorkloadProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkloadProfile{}, &WorkloadProfileList{})
}
