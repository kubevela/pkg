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

package cuex

// Which loops a prepass is allowed to take, kept apart from how it takes
// them.
//
// The mechanism either produces the same answers as the resolver or
// declines, and that is a property of the mechanism. Who is exposed to it
// is a separate decision, and this is the one place that decision is made,
// so changing it later is a line rather than an audit.
//
// The two are separated because the mechanism's risk is silent. A loop
// evaluated one iteration at a time is evaluated in a document built from
// what that iteration reads, and a name carried over incompletely gives
// parameters that are concrete, plausible and not what the template said:
// TestANameCanBeDeclaredMoreThanOnce is one way that happens and there is
// no reason to think it is the only one. So the mechanism has to decline
// wherever it cannot show it has the whole of a name, and the policy has to
// keep the number of templates relying on that judgement small until the
// corpus says it is sound for all of them.

// OptimisePolicy says when the prepass may be used.
type OptimisePolicy struct {
	// Enabled turns it off outright.
	Enabled bool
	// Threshold is the number of iterations below which a loop is left to
	// the resolver.
	//
	// It decides two things. How many templates rely on the prepass being
	// right, since a wrong answer is as wrong on a small loop as a large
	// one; and, because a small loop is where the saving is worth least,
	// whether the saving covers what the prepass costs to run at all.
	//
	// Where the second turns over depends on the shape. With the prepass
	// forced on, a template of chained loops pays from five iterations up,
	// because chaining is where the resolver repeats itself and the prepass
	// does not. One flat loop is the hard case: the resolver does a single
	// round there and is already close to as cheap as it gets, so the
	// prepass is 0.91x at five iterations, level at ten and ahead from
	// twenty.
	//
	// A loop under the threshold is not free, and that is what sets the
	// number rather than the crossover. The threshold is in iterations, so
	// the loop's source is evaluated before it can be applied, and looking
	// and then declining costs a few percent of a small render. That is
	// paid whatever the threshold is, so a higher one pays it on more
	// templates and gives up the saving on them as well.
	Threshold int
	// Batch is how many iterations are answered in one document.
	//
	// It is the whole of the trade. One at a time bounds what is in
	// memory to a single call and pays a document's fixed cost for every
	// one of them; all at once pays that cost once and holds every call
	// the loop makes, which is what the resolver already does and what
	// this exists to avoid.
	//
	// Peak turns out to move little with it, 3.6x to 4.3x over a
	// thousandfold range; time is what it buys, and only up to about ten.
	Batch int
}

// defaultBatch is past where dividing a document's fixed cost stops
// buying anything, and well short of where holding a run starts to cost.
const defaultBatch = 100

// defaultThreshold is where the saving starts being worth the exposure.
//
// Raising it does not buy what it looks like it buys. A loop under the
// threshold still costs what the prepass spends deciding, so a higher one
// pays that on more templates and gives up the saving on them too: at
// twenty iterations, chained four ways, taking the loop is 2.40x and
// declining it is 0.97x.
const defaultThreshold = 10

// batch is the run size to answer in, never zero.
func (p OptimisePolicy) batch() int {
	if p.Batch > 0 {
		return p.Batch
	}
	return defaultBatch
}

// DefaultOptimisePolicy takes any loop of ten iterations or more.
//
// Every CUE file in this workspace, three thousand three hundred of
// them, has no loop of provider calls in it, so this fires on nothing
// that ships and what it is worth belongs entirely to templates people
// write. Those are the ones looping over a thousand resources, and they
// are the ones already in trouble.
var DefaultOptimisePolicy = OptimisePolicy{
	Enabled:   true,
	Threshold: defaultThreshold,
	Batch:     defaultBatch,
}

// allows reports whether a loop of n iterations may be taken.
func (p OptimisePolicy) allows(n int) bool {
	return p.Enabled && n >= p.Threshold
}
