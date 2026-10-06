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
	// the resolver, where the file has more than one round of calls.
	//
	// It decides two things. How many templates rely on the prepass being
	// right, since a wrong answer is as wrong on a small loop as a large
	// one; and, because a small loop is where the saving is worth least,
	// whether the saving covers what the prepass costs to run at all.
	//
	// Chained, the second turns over early: the resolver walks the whole
	// value once per round, so a template of four stages has it doing four
	// passes over something that grows as answers fill in, and that is
	// where it is weak. Ten iterations is 1.48x and twenty five is 1.72x.
	Threshold int
	// LoneThreshold is the same number for a file whose loops read nothing
	// of each other, and it is much higher because such a file is not where
	// the resolver is weak.
	//
	// One round of independent calls is close to as cheap as the resolver
	// gets, and there the prepass is overhead until the loop is big enough
	// for holding every answer at once to cost something. Measured on one
	// flat loop: 0.74x at ten iterations, 0.94x at a hundred, level at a
	// hundred and fifty, 1.07x at two hundred, 1.87x at two thousand.
	//
	// The two numbers cannot be one number. At ten iterations the shapes
	// disagree by a factor of two, 0.74x against 1.48x, so whichever single
	// value were chosen would be wrong for one of them. This was tried with
	// one and the flat shape paid for it.
	//
	// Zero means use Threshold, so a caller naming one number gets that
	// number for both shapes.
	LoneThreshold int
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

// defaultThreshold is where the saving starts being worth the exposure for
// a file with rounds to save.
const defaultThreshold = 10

// defaultLoneThreshold is past where one flat loop turns over, which is
// about a hundred and fifty, with room for the measurement to be noisy.
const defaultLoneThreshold = 200

// loneThreshold is the threshold for a file of one round, falling back to
// the only number a caller gave.
func (p OptimisePolicy) loneThreshold() int {
	if p.LoneThreshold > 0 {
		return p.LoneThreshold
	}
	return p.Threshold
}

// batch is the run size to answer in, never zero.
func (p OptimisePolicy) batch() int {
	if p.Batch > 0 {
		return p.Batch
	}
	return defaultBatch
}

// DefaultOptimisePolicy takes a loop of ten iterations where the file has
// rounds to save, and of two hundred where it has not.
//
// Every CUE file in this workspace, three thousand three hundred of
// them, has no loop of provider calls in it, so this fires on nothing
// that ships and what it is worth belongs entirely to templates people
// write. Those are the ones looping over a thousand resources, and they
// are the ones already in trouble.
var DefaultOptimisePolicy = OptimisePolicy{
	Enabled:       true,
	Threshold:     defaultThreshold,
	LoneThreshold: defaultLoneThreshold,
	Batch:         defaultBatch,
}

// allows reports whether a loop of n iterations may be taken, where
// chained says the file has a loop reading what another produces.
func (p OptimisePolicy) allows(n int, chained bool) bool {
	if !p.Enabled {
		return false
	}
	if chained {
		return n >= p.Threshold
	}
	return n >= p.loneThreshold()
}
