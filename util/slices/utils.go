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

package slices

// Map functional conversion for array items
func Map[T any, V any](arr []T, fn func(T) V) []V {
	_arr := make([]V, len(arr))
	for i, item := range arr {
		_arr[i] = fn(item)
	}
	return _arr
}

// Filter functional filter for array items
func Filter[T any](arr []T, fn func(T) bool) []T {
	var _arr []T
	for _, item := range arr {
		if fn(item) {
			_arr = append(_arr, item)
		}
	}
	return _arr
}

// Index search the index of array item with function
func Index[T any](arr []T, fn func(T) bool) int {
	for idx, item := range arr {
		if fn(item) {
			return idx
		}
	}
	return -1
}

// Find search the first item with function
func Find[T any](arr []T, fn func(T) bool) *T {
	if idx := Index(arr, fn); idx >= 0 {
		return &arr[idx]
	}
	return nil
}

// Flatten the given arr
func Flatten[T any](arr [][]T) []T {
	var _arr []T
	for _, items := range arr {
		for _, item := range items {
			_arr = append(_arr, item)
		}
	}
	return _arr
}
