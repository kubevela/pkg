/*
Copyright 2023 The KubeVela Authors.

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

package apply_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/k8s/apply"
)

func TestDryRunOption(t *testing.T) {
	opt := apply.DryRunOption(true)
	createOptions := &client.CreateOptions{}
	opt.ApplyToCreate(createOptions)
	require.Equal(t, []string{metav1.DryRunAll}, createOptions.DryRun)
	updateOptions := &client.UpdateOptions{}
	opt.ApplyToUpdate(updateOptions)
	require.Equal(t, []string{metav1.DryRunAll}, updateOptions.DryRun)
	patchOptions := &client.PatchOptions{}
	opt.ApplyToPatch(patchOptions)
	require.Equal(t, []string{metav1.DryRunAll}, patchOptions.DryRun)
}
