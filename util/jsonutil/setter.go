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

package jsonutil

// DropField remove field inside a given nested map
func DropField(obj map[string]any, fields ...string) {
	if len(fields) == 0 {
		return
	}
	var cur any = obj
	for _, field := range fields[:len(fields)-1] {
		if next, ok := cur.(map[string]any); ok {
			cur = next[field]
		} else {
			return
		}
	}
	if m, ok := cur.(map[string]any); ok {
		delete(m, fields[len(fields)-1])
	}
}
