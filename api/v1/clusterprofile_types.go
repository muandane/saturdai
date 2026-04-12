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

// ClusterProfileSpec defines the desired state of ClusterProfile.
// +kubebuilder:validation:XValidation:rule="has(self.workloadSelector.labelSelector) || self.workloadSelector.selectAll",message="workloadSelector must specify labelSelector or set selectAll to true"
type ClusterProfileSpec struct {
	// NamespaceSelector filters namespaces where workloads are selected.
	// Nil means all namespaces.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// WorkloadSelector selects Deployments/StatefulSets within matched namespaces.
	WorkloadSelector WorkloadSelector `json:"workloadSelector"`

	// Policy holds the autosizing configuration applied to all matched workloads.
	Policy PolicySpec `json:"policy"`

	// MaxTargets caps the total number of child WorkloadProfiles created. 0 means default (200).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000
	// +optional
	MaxTargets *int32 `json:"maxTargets,omitempty"`
}

// ClusterChildReference identifies a child WorkloadProfile managed by this cluster profile.
type ClusterChildReference struct {
	// Namespace of the child WorkloadProfile.
	Namespace string `json:"namespace"`
	// Name of the child WorkloadProfile.
	Name string `json:"name"`
	// TargetKind is the workload kind (Deployment or StatefulSet).
	TargetKind string `json:"targetKind"`
	// TargetName is the workload name.
	TargetName string `json:"targetName"`
}

// ClusterProfileStatus defines the observed state of ClusterProfile.
type ClusterProfileStatus struct {
	// MatchedNamespaces is the number of namespaces matched by namespaceSelector.
	MatchedNamespaces int32 `json:"matchedNamespaces,omitempty"`

	// ResolvedCount is the total number of workloads matched across all namespaces.
	ResolvedCount int32 `json:"resolvedCount,omitempty"`

	// ActiveChildren is the number of child WorkloadProfiles currently managed.
	ActiveChildren int32 `json:"activeChildren,omitempty"`

	// Children lists child WorkloadProfile references (bounded by MaxTargets).
	// +kubebuilder:validation:MaxItems=1000
	// +optional
	Children []ClusterChildReference `json:"children,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=clusterprofiles,shortName=csp,scope=Cluster
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.policy.mode`
// +kubebuilder:printcolumn:name="Namespaces",type=integer,JSONPath=`.status.matchedNamespaces`
// +kubebuilder:printcolumn:name="Resolved",type=integer,JSONPath=`.status.resolvedCount`
// +kubebuilder:printcolumn:name="Active",type=integer,JSONPath=`.status.activeChildren`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterProfile selects workloads across namespaces and fans out child WorkloadProfiles.
type ClusterProfile struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec ClusterProfileSpec `json:"spec"`

	// +optional
	Status ClusterProfileStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ClusterProfileList contains a list of ClusterProfile.
type ClusterProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ClusterProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterProfile{}, &ClusterProfileList{})
}
