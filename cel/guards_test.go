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
	"encoding/json"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/kubevela/pkg/cel/template"
)

// The default form. `*read | fallback` has no CEL equivalent; has() plus a ternary
// is the replacement, so the capability survives even though the spelling changes.
func TestOptionalDefaults(t *testing.T) {
	cc := cuecontext.New()
	v := cc.CompileString(`s: {host: string, note?: string, data: [string]: string}`).
		LookupPath(cue.ParsePath("s"))
	env, err := typedEnv(map[string]cue.Value{"cfg": v}, nil)
	if err != nil {
		t.Fatal(err)
	}

	in := map[string]interface{}{
		"source":  map[string]interface{}{"cfg": map[string]interface{}{"host": "h", "data": map[string]interface{}{}}},
		"context": map[string]interface{}{},
	}

	for _, tc := range []struct {
		expr string
		want interface{}
	}{
		{`has(source.cfg.note) ? source.cfg.note : "none"`, "none"},
		{`source.cfg.data["absent"]`, nil}, // reading an absent map key errors
	} {
		got, err := fixture.Eval(env, tc.expr, in)
		if tc.want == nil {
			if err == nil {
				t.Errorf("%-42s should have failed, got %#v", tc.expr, got)
			} else {
				t.Logf("%-42s errors as expected: absent map key", tc.expr)
			}
			continue
		}
		if err != nil {
			t.Errorf("%-42s ERROR %v", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%-42s got %#v, want %#v", tc.expr, got, tc.want)
			continue
		}
		t.Logf("%-42s -> %#v", tc.expr, got)
	}
}

// The undefended-read rule, which CEL's checker does not give us.
//
// An unguarded read of an optional field compiles cleanly as its declared type
// and then fails at render with "no such key", so the rule the CUE path enforces
// has to be enforced here too - from the AST rather than from the type.
func TestUndefendedReads(t *testing.T) {
	cc := cuecontext.New()
	v := cc.CompileString(`s: {host: string, note?: string, data: [string]: string}`).
		LookupPath(cue.ParsePath("s"))
	env, err := typedEnv(map[string]cue.Value{"cfg": v}, nil)
	if err != nil {
		t.Fatal(err)
	}
	optional := func(r template.Reference) bool {
		p := strings.Join(r.Path, ".")
		return p == "cfg.note" || strings.HasPrefix(p, "cfg.data.")
	}

	for _, tc := range []struct {
		expr string
		want bool // true = must be flagged
	}{
		{`source.cfg.host`, false},
		{`source.cfg.note`, true},
		{`source.cfg.data["image"]`, true},
		{`source.cfg.host + source.cfg.note`, true},
		{`has(source.cfg.note) ? source.cfg.note : "none"`, false},
		{`has(source.cfg.data.image) ? source.cfg.data.image : "nginx"`, false},
		// has() is a macro expanding to a test-only select, so the guard is on the
		// read itself rather than on an enclosing call.
		{`has(source.cfg.note)`, false},
		// A ternary *condition* is always evaluated, so a read there is not
		// guarded. Getting this wrong is a false negative in the safety check.
		{`source.cfg.note == "x" ? "a" : "b"`, true},
	} {
		bad, err := undefendedReads(env, tc.expr, optional)
		if err != nil {
			t.Errorf("%-52s ERROR %v", tc.expr, err)
			continue
		}
		if got := len(bad) > 0; got != tc.want {
			t.Errorf("%-52s flagged=%v, want %v (%v)", tc.expr, got, tc.want, bad)
			continue
		}
		t.Logf("%-52s flagged=%v", tc.expr, len(bad) > 0)
	}
}

// A substituted value has to end up in an Application's properties, so whatever
// an expression returns must be ordinary Go data that marshals.
//
// ref.Val.Value() is only shallow: a field selection returns what was put in, but
// anything CEL *constructs* is built from its own types, so `{"a": x}` yields
// map[ref.Val]ref.Val and `[x, y]` yields []ref.Val. Neither survives JSON.
func TestNativeValues(t *testing.T) {
	env := testEnv(t)
	in := map[string]interface{}{
		"source": map[string]interface{}{"cfg": map[string]interface{}{
			"host": "db", "port": 5432, "replicas": 6, "secure": true, "tier": "gold",
			"meta": map[string]interface{}{"region": "eu-west", "zone": "z"},
			"data": map[string]interface{}{"image": "nginx:1.25", "tag": "v1"},
		}},
		"context": map[string]interface{}{"appName": "a", "namespace": "n", "cluster": "c"},
	}

	for _, tc := range []struct{ expr, want string }{
		{`source.cfg.meta`, `{"region":"eu-west","zone":"z"}`},
		{`source.cfg.data`, `{"image":"nginx:1.25","tag":"v1"}`},
		{`{"a": source.cfg.host, "b": source.cfg.port}`, `{"a":"db","b":5432}`},
		{`[source.cfg.host, source.cfg.tier]`, `["db","gold"]`},
		{`{"nested": {"deep": [source.cfg.port]}}`, `{"nested":{"deep":[5432]}}`},
		// Iterating a map is covered by TestMapIterationOrderIsNotStable, which
		// does not assert an order - this one did, and flaked on it.
	} {
		v, err := fixture.Eval(env, tc.expr, in)
		if err != nil {
			t.Errorf("%-52s ERROR %v", tc.expr, err)
			continue
		}
		j, err := json.Marshal(v)
		if err != nil {
			t.Errorf("%-52s %T does not marshal: %v", tc.expr, v, err)
			continue
		}
		if string(j) != tc.want {
			t.Errorf("%-52s got %s, want %s", tc.expr, j, tc.want)
			continue
		}
		t.Logf("%-52s %s", tc.expr, j)
	}
}

// The guard detection, probed adversarially.
//
// It walks the AST, so formatting cannot fool it. Asking only "is this read
// inside a ternary arm" is wrong in the unsafe direction twice over, and these
// are the cases that say so.
func TestGuardResilience(t *testing.T) {
	cc := cuecontext.New()
	v := cc.CompileString(`s: {host: string, note?: string, other?: string}`).
		LookupPath(cue.ParsePath("s"))
	env, err := typedEnv(map[string]cue.Value{"cfg": v}, nil)
	if err != nil {
		t.Fatal(err)
	}
	optional := func(r template.Reference) bool {
		p := strings.Join(r.Path, ".")
		return p == "cfg.note" || p == "cfg.other"
	}

	for _, tc := range []struct {
		expr    string
		flagged bool
		why     string
	}{
		{`source.cfg.host`, false, "not optional"},
		{`source.cfg.note`, true, "bare optional read"},
		{`has(source.cfg.note) ? source.cfg.note : "x"`, false, "guarded"},

		// Formatting is irrelevant - this is an AST walk, not string matching.
		{`has( source.cfg.note )?source.cfg.note:"x"`, false, "whitespace"},
		{"has(source.cfg.note)\n ? source.cfg.note\n : \"x\"", false, "newlines"},
		{`(has(source.cfg.note)) ? (source.cfg.note) : ("x")`, false, "parens"},

		// The guard has to test THIS path. Testing a sibling defends nothing, and
		// treating it as a guard let a read through that fails at render.
		{`has(source.cfg.other) ? source.cfg.note : "x"`, true, "guard tests another field"},

		// A read in a condition is always evaluated. Nesting must not launder it:
		// the inner condition sits inside the outer ternary's arm.
		{`true ? (source.cfg.note == "a" ? "x" : "y") : "z"`, true, "nested condition"},

		// Guarded once does not defend a second, bare read.
		{`(has(source.cfg.note) ? source.cfg.note : "x") + source.cfg.note`, true, "second read bare"},

		// CEL's logical operators absorb an error from one side when the other
		// settles the result, so this is a real guard.
		{`has(source.cfg.note) && source.cfg.note == "a"`, false, "&& short-circuit"},
	} {
		bad, err := undefendedReads(env, tc.expr, optional)
		if err != nil {
			t.Errorf("%-58s ERROR %v", tc.expr, err)
			continue
		}
		if got := len(bad) > 0; got != tc.flagged {
			t.Errorf("%-58s flagged=%v, want %v (%s)",
				strings.ReplaceAll(tc.expr, "\n", "\\n"), got, tc.flagged, tc.why)
			continue
		}
		t.Logf("%-58s flagged=%-5v %s", strings.ReplaceAll(tc.expr, "\n", "\\n"), tc.flagged, tc.why)
	}
}
