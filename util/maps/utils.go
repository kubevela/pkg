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

package maps

// Keys return the arr of keys in the given map
func Keys[K comparable, V any](m map[K]V) []K {
	var s []K
	for k, _ := range m {
		s = append(s, k)
	}
	return s
}

// Values return the arr of values in the given map
func Values[K comparable, V any](m map[K]V) []V {
	var s []V
	for _, v := range m {
		s = append(s, v)
	}
	return s
}
