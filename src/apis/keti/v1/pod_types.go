package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
)
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Pod is the Schema for the pods API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=pods,scope=Namespaced
type Pod struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	KubePod           corev1.Pod `json:"kubepod,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodList contains a list of Pod
type PodList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Pod `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pod{}, &PodList{})
}
