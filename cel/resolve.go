/*
Copyright 2026 The KubeVela Authors.

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
package cel

import (
	"context"
	"errors"

	"github.com/kubevela/pkg/cel/template"
)

// A Read is one reference an expression makes, and the property it feeds.
type Read struct {
	template.Reference
	// Property is where the expression sits in the tree, as template.Walk names it.
	Property string
}

// A Resolver answers every read of one root in one tree, before any expression
// is evaluated. Whatever I/O a root needs happens here, never inside an
// expression.
//
// Its value is JSON-shaped, as a decoded document is: map[string]interface{},
// []interface{} and scalars. Numbers are normalised for CEL only within those
// containers.
type Resolver interface {
	Resolve(ctx context.Context, reads []Read) (interface{}, error)
}

// ResolverFunc adapts a function to a Resolver.
type ResolverFunc func(ctx context.Context, reads []Read) (interface{}, error)

// Resolve calls f.
func (f ResolverFunc) Resolve(ctx context.Context, reads []Read) (interface{}, error) {
	return f(ctx, reads)
}

// Static answers every read from a value already in hand.
func Static(v interface{}) Resolver {
	return ResolverFunc(func(context.Context, []Read) (interface{}, error) { return v, nil })
}

// ErrNotReady marks a resolver's error as waiting rather than failing: what a
// read names does not exist yet. A resolver wraps it; a host checks it with
// errors.Is and tries again later rather than reporting a fault.
var ErrNotReady = errors.New("not ready")

// Unknown is what a resolver answers when its root has no value in this
// evaluation, as in a dry run. TreeOptions.Unknown renders reads of it.
var Unknown interface{} = unknownValue{}

type unknownValue struct{}

// TreeOptions adjusts Plan.Eval and EvalTree.
type TreeOptions struct {
	// Unknown renders an expression that reads a root answered Unknown. whole
	// reports whether the expression is the property's entire value; otherwise
	// the result is joined into the surrounding text. Without it, such a read is
	// an error.
	Unknown func(expr string, whole bool) (interface{}, error)
	// OnEvalError is given each evaluation error with the roots the expression
	// reads, each once, and returns the error to report; nil keeps the original. A host uses it to treat a value
	// missing below a read it would wait on, such as an element a status fills
	// in later, as a wait.
	OnEvalError func(expr string, roots []string, err error) error
}
