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

package template

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantExprs []string
		wantWhole bool
		wantErr   string
	}{
		{
			name: "a plain value is untouched",
			raw:  "just-a-string",
		},
		{
			name:      "a whole value is one expression",
			raw:       `$(source["my-source"].region)`,
			wantExprs: []string{`source["my-source"].region`},
			wantWhole: true,
		},
		{
			name:      "an embedded expression keeps its surrounding text",
			raw:       `prefix-$(source["my-source"].region)-suffix`,
			wantExprs: []string{`source["my-source"].region`},
		},
		{
			name:      "several expressions in one value",
			raw:       `$(source["my-source"].region)/$(source["other"].name)`,
			wantExprs: []string{`source["my-source"].region`, `source["other"].name`},
		},
		{
			// Without an escape a value that genuinely contains the delimiter
			// could not be written at all.
			name: "the delimiter can be escaped",
			raw:  `literal $$(not-an-expression)`,
		},
		{
			name:      "parens inside the expression are balanced",
			raw:       `$(strings.Join(["a"], ")"))`,
			wantExprs: []string{`strings.Join(["a"], ")")`},
			wantWhole: true,
		},
		{
			name:    "an unterminated expression is an error, not a literal",
			raw:     `$(source["my-source"].region`,
			wantErr: "unterminated",
		},
		{
			name:    "an empty expression is an error",
			raw:     `$()`,
			wantErr: "empty expression",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected an error mentioning %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var exprs []string
			for _, f := range got.Fragments {
				if f.IsExpr() {
					exprs = append(exprs, f.Expr)
				}
			}
			if len(exprs) != len(tc.wantExprs) {
				t.Fatalf("expected expressions %v, got %v", tc.wantExprs, exprs)
			}
			for i := range exprs {
				if exprs[i] != tc.wantExprs[i] {
					t.Fatalf("expected expressions %v, got %v", tc.wantExprs, exprs)
				}
			}
			if got.Whole() != tc.wantWhole {
				t.Errorf("Whole() = %v, want %v", got.Whole(), tc.wantWhole)
			}
		})
	}
}

// Parse skips its scan for a value holding no "$(", which is almost every
// property. That shortcut has to produce what the scan would have produced.
func TestParseNoExpressionShortcut(t *testing.T) {
	for _, raw := range []string{"", "just-a-string", "a: b, c", "100%", "$notanexpr", "()"} {
		got, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse(%q): %v", raw, err)
		}
		if got.HasExpr() {
			t.Errorf("Parse(%q) found an expression", raw)
		}
		var lit string
		for _, f := range got.Fragments {
			lit += f.Text
		}
		if lit != raw {
			t.Errorf("Parse(%q) reassembles to %q", raw, lit)
		}
	}
}
