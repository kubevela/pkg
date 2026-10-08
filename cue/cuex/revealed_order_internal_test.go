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

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// The calls a workflow step writes run in the order it writes them unless one
// reads another. A step writes a #Fail guarded on a check ahead of a wait, and
// the wait ends the step, so a call that runs after the wait never runs at all.
// Here the wait refuses, which ends the resolve the same way.
func TestCallsRunInTemplateOrder(t *testing.T) {
	for name, tc := range map[string]struct {
		template string
		want     []string
	}{
		// the wait reads the check, so it is a level after it
		"a revealed call ahead of one reading the same call": {`
check: rec.#Do & {$params: "failed"}
if check.$returns == "failed" {
	fail: rec.#Do & {$params: "fail"}
}
wait: rec.#Do & {$params: "wait-" + check.$returns}
`, []string{"failed", "fail", "wait-failed"}},
		// the wait reads nothing, so it is in the check's own level
		"a revealed call ahead of one reading nothing": {`
check: rec.#Do & {$params: "failed"}
if check.$returns == "failed" {
	fail: rec.#Do & {$params: "fail"}
}
wait: rec.#Do & {$params: "wait-now"}
`, []string{"failed", "fail", "wait-now"}},
		// a default is a value the search cannot rule out changing, so the
		// call is ordered by where it is written rather than held for every
		// call there is
		"a call reading a default ahead of calls after it": {`
_image: *"img" | string
job:  rec.#Do & {$params: "job-" + _image}
log:  rec.#Do & {$params: "log"}
wait: rec.#Do & {$params: "wait-now"}
`, []string{"job-img", "log", "wait-now"}},
		"a call reading a default, in a value that can reveal calls": {`
_image: *"img" | string
job: rec.#Do & {$params: "job-" + _image}
log: rec.#Do & {$params: "log"}
if job.$returns == "never" {
	fail: rec.#Do & {$params: "fail"}
}
wait: rec.#Do & {$params: "wait-now"}
`, []string{"job-img", "log", "wait-now"}},
		// the guard reads the check through a field that names its result
		"a call revealed through a named result": {`
check:  rec.#Do & {$params: "failed"}
result: check.$returns
if result == "failed" {
	fail: rec.#Do & {$params: "fail"}
}
wait: rec.#Do & {$params: "wait-now"}
`, []string{"failed", "fail", "wait-now"}},
		// a call under a label that cannot be made until the check answers
		"a call under a label computed from a result": {`
check: rec.#Do & {$params: "failed"}
"\(check.$returns)": rec.#Do & {$params: "fail"}
wait: rec.#Do & {$params: "wait-now"}
`, []string{"failed", "fail", "wait-now"}},
		// the guarded body copies a call a let holds
		"a call a let holds, revealed by a guard": {`
check: rec.#Do & {$params: "failed"}
let next = rec.#Do & {$params: "fail"}
if check.$returns == "failed" {
	fail: next
}
wait: rec.#Do & {$params: "wait-now"}
`, []string{"failed", "fail", "wait-now"}},
		// the fail waits for the probe; the wait, written after it, reads
		// nothing, and must not run ahead of it. Named apart from #Step's
		// check, which names are matched against.
		"a call after one that waits does not overtake it": {`
probe: rec.#Do & {$params: "failed"}
msg:   probe.$returns
fail:  rec.#Do & {$params: msg}
wait:  rec.#Do & {$params: "wait-now"}
`, []string{"failed", "failed", "wait-now"}},
		// what the fail reads is written after it: the producer still runs
		// first, and the wait still comes last
		"a reader ahead of what it reads, then a wait": {`
fail:  rec.#Do & {$params: probe.$returns}
probe: rec.#Do & {$params: "failed"}
wait:  rec.#Do & {$params: "wait-now"}
`, []string{"failed", "failed", "wait-now"}},
		// a waits for c, which waits for b, written between them: b and then c
		// run ahead of a, and the wait still comes last
		"a chain written out of order, then a wait": {`
a:    rec.#Do & {$params: c.$returns}
b:    rec.#Do & {$params: "b"}
c:    rec.#Do & {$params: b.$returns}
wait: rec.#Do & {$params: "wait-now"}
`, []string{"b", "b", "b", "wait-now"}},
		// only what the first held call waits for may overtake it: y feeds a
		// later held call, not a, so it waits its turn, and a runs before y
		// can end the resolve
		"a producer for a later held call does not overtake an earlier one": {`
a: rec.#Do & {$params: r.$returns}
b: rec.#Do & {$params: y.$returns}
y: rec.#Do & {$params: "stop"}
r: rec.#Do & {$params: "r"}
`, []string{"r", "r", "stop"}},
		// a definition can name a result as well as a field can
		"a call revealed through a definition naming a result": {`
probe:   rec.#Do & {$params: "failed"}
#result: probe.$returns
if #result == "failed" {
	fail: rec.#Do & {$params: "fail"}
}
wait: rec.#Do & {$params: "wait-now"}
`, []string{"failed", "fail", "wait-now"}},
		// the result is read through a let, which the reference search
		// cannot follow and which CUE renders as concrete before the call
		// it reads has run: the call still runs after what it reads
		"a call reading a result through a let": {`
l: [rec.#Do & {$params: "a"}, rec.#Do & {$params: "b"}]
n: l[1].$returns
let v = n
c: rec.#Do & {$params: "s-\(v)"}
stop: rec.#Do & {$params: "stop"}
`, []string{"a", "b", "s-b", "stop"}},
		// the let reads a call written after the reader: that call still
		// runs first
		"a call reading a later call through a let": {`
let v = n
c: rec.#Do & {$params: "s-\(v)"}
n: l.$returns
l: rec.#Do & {$params: "b"}
stop: rec.#Do & {$params: "stop"}
`, []string{"b", "s-b", "stop"}},
		// a field elsewhere shares the name the let reads: it is not what the
		// let reads, and its call, which ends the resolve, waits its turn
		"a let read, beside an unrelated field of the same name": {`
let v = n
c: rec.#Do & {$params: "s-\(v)"}
other: n: rec.#Do & {$params: "stop"}
n: l.$returns
l: rec.#Do & {$params: "b"}
`, []string{"b", "s-b", "stop"}},
		// the let, its reader and its producer sit in a struct unified with a
		// definition, as a step's fields usually do
		"a let read inside a unified struct": {`
#D: {...}
x: #D & {
	let v = n
	c: rec.#Do & {$params: "s-\(v)"}
	n: l.$returns
	l: rec.#Do & {$params: "b"}
}
stop: rec.#Do & {$params: "stop"}
`, []string{"b", "s-b", "stop"}},
		"a let read inside a parenthesised struct": {`
x: ({
	let v = n
	c: rec.#Do & {$params: "s-\(v)"}
	n: l.$returns
	l: rec.#Do & {$params: "b"}
})
stop: rec.#Do & {$params: "stop"}
`, []string{"b", "s-b", "stop"}},
		// the comprehension is the package's, as a workflow step's often is
		"a call a package's definition reveals": {`
step: rec.#Step
`, []string{"failed", "fail", "wait-now"}},
	} {
		t.Run(name, func(t *testing.T) {
			rec := &recorder{failOn: tc.want[len(tc.want)-1]}
			c := NewCompilerWithInternalPackages(rec.pkg())
			_, err := c.CompileString(context.Background(), "import \"vela/rec\"\n"+tc.template)
			require.Error(t, err, "the wait ends the resolve")
			require.Equal(t, tc.want, rec.order())
		})
	}
}

// A disjunction that only settles once a call has answered can hold a call of
// its own, which is there to run once it does.
func TestACallADisjunctionRevealsRuns(t *testing.T) {
	rec := &recorder{}
	c := NewCompilerWithInternalPackages(rec.pkg())
	_, err := c.CompileString(context.Background(), `
import "vela/rec"

check: rec.#Do & {$params: "x"}
out: {kind: check.$returns} & ({kind: "x", next: rec.#Do & {$params: "next"}} | {kind: "y"})
`)
	require.NoError(t, err)
	require.Equal(t, []string{"x", "next"}, rec.order())
}
