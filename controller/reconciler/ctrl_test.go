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

package reconciler_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/kubevela/pkg/controller/reconciler"
	"github.com/kubevela/pkg/util/k8s"
)

func TestShouldSkipReconcile(t *testing.T) {
	cm := &corev1.ConfigMap{}
	require.False(t, reconciler.ShouldSkipReconcile(cm))
	require.NoError(t, k8s.AddLabel(cm, reconciler.LabelSkipReconcile, reconciler.ValueTrue))
	require.True(t, reconciler.ShouldSkipReconcile(cm))
}
