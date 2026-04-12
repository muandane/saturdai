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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadSelector selects Deployments and/or StatefulSets in the profile namespace.
// +kubebuilder:validation:XValidation:rule="!has(self.kinds) || ((!has(self.kinds.deployment) || self.kinds.deployment) || (!has(self.kinds.statefulSet) || self.kinds.statefulSet))",message="kinds must not disable both deployment and statefulSet"
type WorkloadSelector struct {
	// LabelSelector filters workloads by labels. Nil means no label filter (must set SelectAll).
	// +optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`

	// SelectAll opts in to selecting every Deployment and StatefulSet in the namespace.
	// Requires explicit opt-in because of the high blast radius.
	// +kubebuilder:default=false
	// +optional
	SelectAll bool `json:"selectAll,omitempty"`

	// Kinds restricts which workload types are selected. Both default to true when omitted.
	// +optional
	Kinds *WorkloadKinds `json:"kinds,omitempty"`
}

// WorkloadKinds controls which workload types a selector matches.
type WorkloadKinds struct {
	// Deployment enables selection of Deployments.
	// +kubebuilder:default=true
	// +optional
	Deployment *bool `json:"deployment,omitempty"`

	// StatefulSet enables selection of StatefulSets.
	// +kubebuilder:default=true
	// +optional
	StatefulSet *bool `json:"statefulSet,omitempty"`
}

// PolicySpec holds the shared autosizing policy fields used by NamespaceProfile and ClusterProfile.
type PolicySpec struct {
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

// NamespaceProfileSpec defines the desired state of NamespaceProfile.
// +kubebuilder:validation:XValidation:rule="has(self.workloadSelector.labelSelector) || self.workloadSelector.selectAll",message="workloadSelector must specify labelSelector or set selectAll to true"
type NamespaceProfileSpec struct {
	// WorkloadSelector selects Deployments/StatefulSets in the profile namespace.
	WorkloadSelector WorkloadSelector `json:"workloadSelector"`

	// Policy holds the autosizing configuration applied to all matched workloads.
	Policy PolicySpec `json:"policy"`

	// MaxTargets caps the number of child WorkloadProfiles created. 0 means default (50).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=200
	// +optional
	MaxTargets *int32 `json:"maxTargets,omitempty"`
}

// ChildReference identifies a child WorkloadProfile managed by this profile.
type ChildReference struct {
	// Name of the child WorkloadProfile.
	Name string `json:"name"`
	// TargetKind is the workload kind (Deployment or StatefulSet).
	TargetKind string `json:"targetKind"`
	// TargetName is the workload name.
	TargetName string `json:"targetName"`
}

// NamespaceProfileStatus defines the observed state of NamespaceProfile.
type NamespaceProfileStatus struct {
	// ResolvedCount is the number of workloads matched by the selector.
	ResolvedCount int32 `json:"resolvedCount,omitempty"`

	// ActiveChildren is the number of child WorkloadProfiles currently managed.
	ActiveChildren int32 `json:"activeChildren,omitempty"`

	// Children lists child WorkloadProfile references (bounded by MaxTargets).
	// +kubebuilder:validation:MaxItems=200
	// +optional
	Children []ChildReference `json:"children,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=namespaceprofiles,shortName=nsp
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.policy.mode`
// +kubebuilder:printcolumn:name="Resolved",type=integer,JSONPath=`.status.resolvedCount`
// +kubebuilder:printcolumn:name="Active",type=integer,JSONPath=`.status.activeChildren`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NamespaceProfile selects workloads in its namespace and fans out child WorkloadProfiles.
type NamespaceProfile struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec NamespaceProfileSpec `json:"spec"`

	// +optional
	Status NamespaceProfileStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// NamespaceProfileList contains a list of NamespaceProfile.
type NamespaceProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NamespaceProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NamespaceProfile{}, &NamespaceProfileList{})
}
