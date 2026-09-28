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
	"testing"

	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
)

// The build is the largest phase and the template's text is the same on every
// render, so which half of it is which decides whether any of it keeps.
func BenchmarkParseAgainstCompile(b *testing.B) {
	c := NewCompilerWithDefaultInternalPackages()
	imports := c.PackageManager.GetImports()
	src := apportionSrc(8)

	b.Run("parse", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := parser.ParseFile("-", src, parser.ParseComments); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("parse and compile", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			f, err := parser.ParseFile("-", src, parser.ParseComments)
			if err != nil {
				b.Fatal(err)
			}
			bi := build.NewContext().NewInstance("", nil)
			bi.Imports = imports
			if err := bi.AddSyntax(f); err != nil {
				b.Fatal(err)
			}
			if v := cuecontext.New().BuildInstance(bi); v.Err() != nil {
				b.Fatal(v.Err())
			}
		}
	})
	b.Run("read the syntax the way the compiler does", func(b *testing.B) {
		f, err := parser.ParseFile("-", src, parser.ParseComments)
		if err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = mayGrowArcs(f)
			_ = scopeOf(f, imports)
		}
	})
}

// A definition's text is the same on every render, so the parse looks like
// something to keep. It is not: compiling writes into the syntax what each
// reference resolved to, so two builds from one parsed file race, and the
// value handed back goes on pointing at those nodes, which rules out handing
// the file to the next render when this one looks done. A parse per render is
// the price of that.
