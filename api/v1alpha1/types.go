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
	EBPFConfig           *EBPFConfig           `json:"ebpfConfig,omitempty"`
}

// +k8s:deepcopy-gen=true
type PodConfigMapConfigStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen=true
type EBPFConfig struct {
	SyscallMonitoring *SyscallMonitoringConfig `json:"syscallMonitoring,omitempty"`
	L4Firewall        *L4FirewallConfig        `json:"l4Firewall,omitempty"`
	MetricsExport     *MetricsExportConfig     `json:"metricsExport,omitempty"`
}

// +k8s:deepcopy-gen=true
type SyscallMonitoringConfig struct {
	Enabled      bool     `json:"enabled"`
	SyscallNames []string `json:"syscallNames,omitempty"`
}

// +k8s:deepcopy-gen=true
type L4FirewallConfig struct {
	Enabled       bool              `json:"enabled"`
	AllowedPorts  []int32           `json:"allowedPorts,omitempty"`
	BlockedPorts  []int32           `json:"blockedPorts,omitempty"`
	DefaultAction L4FirewallAction  `json:"defaultAction"`
}

// +k8s:deepcopy-gen=true
type L4FirewallAction string

const (
	L4FirewallActionAllow L4FirewallAction = "allow"
	L4FirewallActionBlock L4FirewallAction = "block"
)

// +k8s:deepcopy-gen=true
type MetricsExportConfig struct {
	Enabled        bool   `json:"enabled"`
	UpdateInterval string `json:"updateInterval,omitempty"`
}

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PodConfigMapConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodConfigMapConfig `json:"items"`
}
