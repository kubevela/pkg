package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
)

// +kubebuilder:object:root=true

// Workflow is the Schema for the workflow API
// +kubebuilder:storageversion
// +kubebuilder:resource:categories={oam}
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Workflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Mode                          *workflowv1alpha1.WorkflowExecuteMode `json:"mode,omitempty"`
	workflowv1alpha1.WorkflowSpec `json:",inline"`
}

// +kubebuilder:object:root=true

// WorkflowList contains a list of Workflow
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type WorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workflow `json:"items"`
}
