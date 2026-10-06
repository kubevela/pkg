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

	"cuelang.org/go/cue/format"
)

// PrepassWideOpenForTest runs the prepass over every loop a template has,
// whatever its size and whether or not it asked, and returns the template
// it would hand on.
//
// Exported only to the tests, and only so the corpus the resolver is
// tested against can be pointed at the prepass too: that corpus lives in
// the external test package and the prepass does not.
//
// Wide open on purpose. The default policy is opt in and over a threshold,
// and a corpus run under the default would be testing the policy rather
// than the mechanism it is there to contain.
func (in *Compiler) PrepassWideOpenForTest(ctx context.Context, src string) (string, int, bool) {
	out, took := in.prepass(ctx, src, in.PackageManager.GetImports(),
		OptimisePolicy{Enabled: true, Threshold: 1})
	if !took {
		return src, 0, false
	}
	bs, err := format.Node(out.file)
	if err != nil {
		return src, 0, false
	}
	return string(bs), out.calls, true
}
