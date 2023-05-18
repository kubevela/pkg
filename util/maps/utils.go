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
	for k := range m {
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

// Map functional conversion for map items
func Map[K comparable, U any, V any](m map[K]U, fn func(U) V) map[K]V {
	_m := make(map[K]V, len(m))
	for key, val := range m {
		_m[key] = fn(val)
	}
	return _m
}

// Filter functional filter for map items
func Filter[K comparable, V any](m map[K]V, fn func(K, V) bool) map[K]V {
	_m := make(map[K]V, len(m))
	for key, val := range m {
		if fn(key, val) {
			_m[key] = val
		}
	}
	return _m
}

// Copy return a copy of given map
func Copy[K comparable, V any](m map[K]V) map[K]V {
	_m := make(map[K]V)
	for k, v := range m {
		_m[k] = v
	}
	return _m
}
