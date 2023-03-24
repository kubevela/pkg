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

package reconciler

import (
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kubevela/pkg/util/k8s"
)

const (
	// LabelPause skip reconcile for objects that contains the label and with value "true"
	LabelPause = "controller.core.oam.dev/pause"

	// ValueTrue true value
	ValueTrue = "true"
)

// SetPause set if the target object should skip reconcile
func SetPause(o runtime.Object, skip bool) {
	if skip {
		_ = k8s.AddLabel(o, LabelPause, ValueTrue)
		return
	}
	_ = k8s.DeleteLabel(o, LabelPause)
}

// IsPaused check if the target object should skip reconcile
func IsPaused(o runtime.Object) bool {
	return k8s.GetLabel(o, LabelPause) == ValueTrue
}
