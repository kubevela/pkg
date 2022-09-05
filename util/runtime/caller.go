/*
Copyright 2022 The KubeVela Authors.

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

package runtime

import (
	"regexp"

	"github.com/go-stack/stack"
)

var (
	controllerFilenamePattern = regexp.MustCompile("([a-z]+)_?controller.go")
)

// GetControllerInCaller extract controller name from the stack of callers
// It identifies xxxcontroller.go or xxx_controller.go files in the caller trace
func GetControllerInCaller() string {
	stacktrace := stack.Trace().TrimRuntime().String()
	match := controllerFilenamePattern.FindSubmatch([]byte(stacktrace))
	if len(match) > 1 {
		return string(match[1])
	}
	return ""
}
