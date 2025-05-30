package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PodConfigMapConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PodConfigMapConfigSpec   `json:"spec,omitempty"`
	Status            PodConfigMapConfigStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen=true
type PodConfigMapConfigSpec struct {
	LabelsToInclude      []string              `json:"labelsToInclude,omitempty"`
	AnnotationsToInclude []string              `json:"annotationsToInclude,omitempty"`
	PodSelector          *metav1.LabelSelector `json:"podSelector,omitempty"`
}

// +k8s:deepcopy-gen=true
type PodConfigMapConfigStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PodConfigMapConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodConfigMapConfig `json:"items"`
}
