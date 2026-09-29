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

// Package debug is a provider function that does nothing, so that what
// surrounds it can be measured.
//
// The question it answers is where a resolve spends its time when the
// functions themselves are not spending any. Each call logs how long it has
// been since the last one returned, and that gap is the resolver: finding the
// call, reading its parameters, putting the answer back, and looking for
// whatever is next. A template that calls this n times turns that gap into
// something readable from a controller's own log, on a real value, rather than
// something inferred from a benchmark.
package debug

import (
	"context"
	"sync"
	"time"

	_ "embed"

	"k8s.io/klog/v2"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
)

// NoopParams is what a call passes in.
type NoopParams struct {
	// Tag names the call in the log, so one can be told from another.
	Tag string `json:"tag,omitempty"`
}

// NoopReturns hands the tag back, so one call can be chained onto another
// without the function doing anything.
type NoopReturns struct {
	Tag string `json:"tag"`
}

// NoopVars .
type NoopVars = providers.Params[NoopParams]

// NoopResult .
type NoopResult = providers.Returns[NoopReturns]

// between is what the gap is measured against, and the clock is in here with
// the rest of it because the resolver may run these calls alongside each
// other: a clock read outside the lock races a test holding it still.
var between = struct {
	sync.Mutex
	clock func() time.Time
	last  time.Time
	calls int
}{clock: time.Now}

// Noop does nothing and says how long it was since the last time it did
// nothing. The gap is what the resolver spent between two calls that spent
// none themselves.
//
// The answer carries no clock reading. A rendered manifest is compared against
// the last one to find what drifted, and a timestamp in the output would make
// every render differ.
func Noop(_ context.Context, in *NoopVars) (*NoopResult, error) {
	between.Lock()
	now := between.clock()
	gap, nth := time.Duration(0), between.calls+1
	if !between.last.IsZero() {
		gap = now.Sub(between.last)
	}
	between.calls = nth
	between.Unlock()

	klog.InfoS("cuex noop call",
		"tag", in.Params.Tag,
		"nth", nth,
		"sinceLastReturn", gap.String())

	// Stamped after the log line rather than before it, so the next gap is
	// the resolver's time and not this function's.
	between.Lock()
	between.last = between.clock()
	between.Unlock()
	return &NoopResult{Returns: NoopReturns{Tag: in.Params.Tag}}, nil
}

// Reset forgets what has been seen, so one measurement does not report the gap
// since another.
func Reset() {
	between.Lock()
	defer between.Unlock()
	between.last = time.Time{}
	between.calls = 0
}

// Calls is how many times Noop has run since the last Reset.
func Calls() int {
	between.Lock()
	defer between.Unlock()
	return between.calls
}

// ProviderName .
const ProviderName = "debug"

//go:embed debug.cue
var template string

// Package .
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"noop": cuexruntime.GenericProviderFn[NoopVars, NoopResult](Noop),
}))
