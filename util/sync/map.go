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

package sync

import "sync"

type Map[K comparable, V any] struct {
	mu   sync.RWMutex
	data map[K]V
}

func NewMap[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{data: map[K]V{}}
}

func (in *Map[K, V]) Get(key K) (V, bool) {
	in.mu.RLock()
	defer in.mu.RUnlock()
	val, found := in.data[key]
	return val, found
}

func (in *Map[K, V]) Set(key K, val V) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.data[key] = val
}

func (in *Map[K, V]) Del(key K) {
	in.mu.Lock()
	defer in.mu.Unlock()
	delete(in.data, key)
}

func (in *Map[K, V]) Data() map[K]V {
	in.mu.RLock()
	in.mu.RUnlock()
	copied := map[K]V{}
	for k, v := range in.data {
		copied[k] = v
	}
	return copied
}
