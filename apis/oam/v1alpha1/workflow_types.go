package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

	Mode         *WorkflowExecuteMode `json:"mode,omitempty"`
	WorkflowSpec `json:",inline"`
}

// +kubebuilder:object:root=true

// WorkflowList contains a list of Workflow
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type WorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workflow `json:"items"`
}

// InputItem defines an input variable of WorkflowStep
type InputItem struct {
	ParameterKey string `json:"parameterKey,omitempty"`
	From         string `json:"from"`
}

// OutputItem defines an output variable of WorkflowStep
type OutputItem struct {
	ValueFrom string `json:"valueFrom"`
	Name      string `json:"name"`
}

// StepOutputs defines output variable of WorkflowStep
type StepOutputs []OutputItem

// StepInputs defines variable input of WorkflowStep
type StepInputs []InputItem

// WorkflowStepMeta contains the meta data of a workflow step
type WorkflowStepMeta struct {
	Alias string `json:"alias,omitempty"`
}

// WorkflowStepBase defines the workflow step base
type WorkflowStepBase struct {
	// Name is the unique name of the workflow step.
	Name string `json:"name,omitempty"`
	// Type is the type of the workflow step.
	Type string `json:"type"`
	// Meta is the meta data of the workflow step.
	Meta *WorkflowStepMeta `json:"meta,omitempty"`
	// If is the if condition of the step
	If string `json:"if,omitempty"`
	// Timeout is the timeout of the step
	Timeout string `json:"timeout,omitempty"`
	// DependsOn is the dependency of the step
	DependsOn []string `json:"dependsOn,omitempty"`
	// Inputs is the inputs of the step
	Inputs StepInputs `json:"inputs,omitempty"`
	// Outputs is the outputs of the step
	Outputs StepOutputs `json:"outputs,omitempty"`

	// Properties is the properties of the step
	// +kubebuilder:pruning:PreserveUnknownFields
	Properties *runtime.RawExtension `json:"properties,omitempty"`
}

// WorkflowStep defines how to execute a workflow step.
type WorkflowStep struct {
	WorkflowStepBase `json:",inline"`
	// Mode is only valid for sub steps, it defines the mode of the sub steps
	// +nullable
	Mode     WorkflowMode       `json:"mode,omitempty"`
	SubSteps []WorkflowStepBase `json:"subSteps,omitempty"`
}

// WorkflowSpec defines workflow steps and other attributes
type WorkflowSpec struct {
	Steps []WorkflowStep `json:"steps,omitempty"`
}

// WorkflowMode describes the mode of workflow
type WorkflowMode string

// WorkflowExecuteMode defines the mode of workflow execution
type WorkflowExecuteMode struct {
	// Steps is the mode of workflow steps execution
	Steps WorkflowMode `json:"steps,omitempty"`
	// SubSteps is the mode of workflow sub steps execution
	SubSteps WorkflowMode `json:"subSteps,omitempty"`
}
