/*
Copyright 2026 The KubeVela Authors.

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

package v1alpha1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func TestForEachRoundTrip(t *testing.T) {
	r := require.New(t)
	in := `
name: scale
type: scale-component
forEach:
  items: [east, 3, {name: west, replicas: 2}]
  mode: dag
`
	var step WorkflowStep
	r.NoError(yaml.Unmarshal([]byte(in), &step))
	r.NotNil(step.ForEach)
	r.Equal(WorkflowMode("dag"), step.ForEach.Mode)
	r.Empty(step.ForEach.From)
	r.JSONEq(`["east", 3, {"name": "west", "replicas": 2}]`, string(step.ForEach.Items.Raw))

	out, err := json.Marshal(step)
	r.NoError(err)
	var again WorkflowStep
	r.NoError(json.Unmarshal(out, &again))
	r.Equal(step, again)
}

func TestForEachHoldsAnExpression(t *testing.T) {
	r := require.New(t)
	var step WorkflowStep
	r.NoError(yaml.Unmarshal([]byte(`{name: s, type: t, forEach: {items: "$(source.inventory.clusters)"}}`), &step))
	r.JSONEq(`"$(source.inventory.clusters)"`, string(step.ForEach.Items.Raw))
}

func TestForEachFrom(t *testing.T) {
	r := require.New(t)
	var step WorkflowStep
	r.NoError(yaml.Unmarshal([]byte(`{name: s, type: t, forEach: {from: regions.names}}`), &step))
	r.Equal("regions.names", step.ForEach.From)
	r.Nil(step.ForEach.Items)
}

func TestWorkflowStepWithoutForEachOmitsIt(t *testing.T) {
	r := require.New(t)
	out, err := json.Marshal(WorkflowStep{WorkflowStepBase: WorkflowStepBase{Name: "s", Type: "t"}})
	r.NoError(err)
	r.NotContains(string(out), "forEach")
}

func TestForEachDeepCopy(t *testing.T) {
	r := require.New(t)
	var step WorkflowStep
	r.NoError(yaml.Unmarshal([]byte(`{name: s, type: t, forEach: {items: [a]}}`), &step))
	copied := step.DeepCopy()
	copied.ForEach.Items.Raw[2] = 'b'
	copied.ForEach.Mode = "step"
	r.JSONEq(`["a"]`, string(step.ForEach.Items.Raw))
	r.Empty(step.ForEach.Mode)
}
