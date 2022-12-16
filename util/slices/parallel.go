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

import (
	"sync"
)

// DefaultParallelism default parallelism for par
const DefaultParallelism = 5

// ParConfig config for par execution
type ParConfig struct {
	parallelism int
}

// NewParConfig build config for par execution
func NewParConfig(opts ...ParOption) *ParConfig {
	cfg := &ParConfig{
		parallelism: DefaultParallelism,
	}
	for _, opt := range opts {
		opt.ApplyTo(cfg)
	}
	return cfg
}

// ParOption options for par execution
type ParOption interface {
	ApplyTo(*ParConfig)
}

// Parallelism specify the parallelism of par execution
type Parallelism int

// ApplyTo .
func (in Parallelism) ApplyTo(cfg *ParConfig) {
	cfg.parallelism = int(in)
}

// ParFor run parallel executions for items in arr
func ParFor[T any](arr []T, fn func(T), opts ...ParOption) {
	cfg := NewParConfig(opts...)
	pool := make(chan struct{}, cfg.parallelism)
	wg := sync.WaitGroup{}
	wg.Add(len(arr))
	for _, item := range arr {
		go func(i T) {
			pool <- struct{}{}
			fn(i)
			<-pool
			wg.Done()
		}(item)
	}
	wg.Wait()
	close(pool)
}

type ordered[V any] struct {
	idx  int
	item V
}

func toOrdered[V any](arr []V) []ordered[V] {
	var _arr []ordered[V]
	for idx, item := range arr {
		_arr = append(_arr, ordered[V]{idx: idx, item: item})
	}
	return _arr
}

// ParMap run parallel mapping functions for arr, returned values are ordered
func ParMap[T any, V any](arr []T, fn func(T) V, opts ...ParOption) []V {
	outs := make(chan ordered[V])
	go ParFor(toOrdered(arr), func(i ordered[T]) {
		outs <- ordered[V]{idx: i.idx, item: fn(i.item)}
	}, opts...)
	_arr := make([]V, len(arr))
	for range arr {
		out := <-outs
		_arr[out.idx] = out.item
	}
	close(outs)
	return _arr
}
