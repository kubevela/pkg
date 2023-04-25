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

import "sync"

// SyncMap defines the thread-safe map
type SyncMap[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

// NewSyncMap create a new SyncMap
func NewSyncMap[K comparable, V any]() *SyncMap[K, V] {
	return &SyncMap[K, V]{
		m: map[K]V{},
	}
}

// NewSyncMapFrom create a new SyncMap from existing map. The data is copied
// instead of directly referring to the given map.
func NewSyncMapFrom[K comparable, V any](in map[K]V) *SyncMap[K, V] {
	out := NewSyncMap[K, V]()
	for k, v := range in {
		out.m[k] = v
	}
	return out
}

// Get return the value for the given key
func (in *SyncMap[K, V]) Get(key K) (V, bool) {
	in.mu.RLock()
	defer in.mu.RUnlock()
	val, found := in.m[key]
	return val, found
}

// Set the value for the given key
func (in *SyncMap[K, V]) Set(key K, val V) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.m[key] = val
}

// Del delete the value for the given key
func (in *SyncMap[K, V]) Del(key K) {
	in.mu.Lock()
	defer in.mu.Unlock()
	delete(in.m, key)
}

// Keys return a copy of keys
func (in *SyncMap[K, V]) Keys() []K {
	in.mu.RLock()
	defer in.mu.RUnlock()
	var keys []K
	for k := range in.m {
		keys = append(keys, k)
	}
	return keys
}

// Values return a copy of values
func (in *SyncMap[K, V]) Values() []V {
	in.mu.RLock()
	defer in.mu.RUnlock()
	var keys []V
	for _, v := range in.m {
		keys = append(keys, v)
	}
	return keys
}

// Range function for ranging over the map, it uses RLock during ranging
func (in *SyncMap[K, V]) Range(f func(K, V)) {
	in.mu.RLock()
	defer in.mu.RUnlock()
	for k, v := range in.m {
		f(k, v)
	}
}

// Load function for reset the underlying map, copies all data from the given map
func (in *SyncMap[K, V]) Load(m map[K]V) {
	_m := Copy(m)
	in.mu.Lock()
	defer in.mu.Unlock()
	in.m = _m
}

// Data return a full copy of underlying
func (in *SyncMap[K, V]) Data() map[K]V {
	in.mu.RLock()
	defer in.mu.RUnlock()
	copied := map[K]V{}
	for k, v := range in.m {
		copied[k] = v
	}
	return copied
}
