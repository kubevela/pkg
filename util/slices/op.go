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

package slices

// Intersect calculate the intersection of two array
func Intersect[T comparable](a, b []T) (arr []T) {
	m := make(map[T]struct{}, len(b))
	for _, item := range b {
		m[item] = struct{}{}
	}
	for _, item := range a {
		if _, found := m[item]; found {
			arr = append(arr, item)
		}
	}
	return arr
}

// Union calculate the union of two array
func Union[T comparable](a, b []T) (arr []T) {
	m := make(map[T]struct{}, len(a))
	for _, item := range a {
		m[item] = struct{}{}
		arr = append(arr, item)
	}
	for _, item := range b {
		if _, found := m[item]; !found {
			arr = append(arr, item)
		}
	}
	return arr
}

// Subtract calculate the subtraction of two array
func Subtract[T comparable](a, b []T) (arr []T) {
	m := make(map[T]struct{}, len(b))
	for _, item := range b {
		m[item] = struct{}{}
	}
	for _, item := range a {
		if _, found := m[item]; !found {
			arr = append(arr, item)
		}
	}
	return arr
}
