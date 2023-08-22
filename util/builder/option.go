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

package builder

// Option the generic type for option type T
type Option[T any] interface {
	ApplyTo(*T)
}

// Constructor the constructor for option interface
type Constructor[T any] interface {
	New() *T
}

// NewOptions create options T with given Option args. If T implements
// Constructor, it will call its construct function to initialize first
func NewOptions[T any](opts ...Option[T]) *T {
	t := new(T)
	if c, ok := any(t).(Constructor[T]); ok {
		t = c.New()
	}
	ApplyTo(t, opts...)
	return t
}

// ApplyTo run all option args for setting the options
func ApplyTo[T any](t *T, opts ...Option[T]) {
	for _, opt := range opts {
		opt.ApplyTo(t)
	}
}

// OptionFn wrapper for ApplyTo function
type OptionFn[T any] func(*T)

// ApplyTo implements Option interface
func (in OptionFn[T]) ApplyTo(t *T) {
	(in)(t)
}
