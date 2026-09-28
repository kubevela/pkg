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

package cuex_test

import (
	"fmt"
	"strings"
)

// resolveCase is one template plus what the resolver is expected to make of it.
type resolveCase struct {
	// Template is compiled with the default internal packages.
	Template string
	// WantErr is a substring of the expected error. Empty means success.
	WantErr string
	// Divergent marks a case where the batching resolver deliberately differs
	// from the pre-batching one, with the reason. Those cases are excluded from
	// the differential comparison and pinned by their own assertions instead.
	Divergent string
}

// resolveCorpus is the shared fixture for the equivalence and golden tests. It
// covers every value shape the resolver walks: nesting, lists, hidden and
// optional fields, definitions, comprehensions, step ordering, and the
// dependency orders that decide when a call is runnable.
var resolveCorpus = map[string]resolveCase{
	"single call": {Template: `
import "vela/base64"
x: base64.#Encode & {$params: "hello"}`},

	"no calls at all": {Template: `
a: "plain"
b: {c: 1, d: [1, 2, 3]}`},

	"two independent calls": {Template: `
import "vela/base64"
a: base64.#Encode & {$params: "a"}
b: base64.#Encode & {$params: "b"}`},

	"back reference, producer declared first": {Template: `
import "vela/base64"
b: base64.#Encode & {$params: "x"}
a: base64.#Encode & {$params: b.$returns}`},

	"chain of four": {Template: `
import "vela/base64"
x0: base64.#Encode & {$params: "seed"}
x1: base64.#Encode & {$params: x0.$returns}
x2: base64.#Encode & {$params: x1.$returns}
x3: base64.#Encode & {$params: x2.$returns}`},

	"round trip encode then decode": {Template: `
import "vela/base64"
enc: base64.#Encode & {$params: "round trip"}
dec: base64.#Decode & {$params: enc.$returns}
out: dec.$returns`},

	"nested structs": {Template: `
import "vela/base64"
outer: middle: inner: base64.#Encode & {$params: "deep"}
sibling: other: base64.#Encode & {$params: "shallow"}`},

	"calls inside a list": {Template: `
import "vela/base64"
xs: [base64.#Encode & {$params: "a"}, base64.#Encode & {$params: "b"}]`},

	"call inside a nested list": {Template: `
import "vela/base64"
xs: [{inner: [base64.#Encode & {$params: "deep"}]}]`},

	"call in a list beside plain items": {Template: `
import "vela/base64"
xs: ["plain", base64.#Encode & {$params: "a"}, 42, {k: "v"}]`},

	"list consuming an earlier list element": {Template: `
import "vela/base64"
xs: [base64.#Encode & {$params: "a"}, base64.#Encode & {$params: xs[0].$returns}]`},

	"hidden field call": {Template: `
import "vela/base64"
_h: base64.#Encode & {$params: "hidden"}
out: _h.$returns`},

	"hidden field call, nested": {Template: `
import "vela/base64"
wrap: {_h: base64.#Encode & {$params: "hidden"}, out: _h.$returns}`},

	"optional field call": {Template: `
import "vela/base64"
o?: base64.#Encode & {$params: "opt"}`},

	"call consumed by a definition": {Template: `
import "vela/base64"
#Shape: {v: string}
c: base64.#Encode & {$params: "x"}
out: #Shape & {v: c.$returns}`},

	"call revealed by a comprehension guard": {Template: `
import "vela/base64"
a: base64.#Encode & {$params: "seed"}
if a.$returns != "" {
  b: base64.#Encode & {$params: a.$returns}
}`},

	"calls ordered by step attribute": {Template: `
import "vela/base64"
z: base64.#Encode & {$params: "z"} @step(1)
a: base64.#Encode & {$params: "a"} @step(2)
m: base64.#Encode & {$params: "m"} @step(3)`},

	"step ordering with a dependency": {Template: `
import "vela/base64"
second: base64.#Encode & {$params: first.$returns} @step(2)
first: base64.#Encode & {$params: "start"} @step(1)`},

	"call with no $params": {Template: `
x: {#do: "encode", #provider: "base64"}`},

	"native provider, vela/cue strategyUnify": {Template: `
import "vela/cue"
secret: {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {name: "ip", namespace: "default"}
}
patch: cue.#StrategyUnify & {
	$params: {
		value: secret
		patch: stringData: ip: "127.0.0.1"
	}
}`},

	"native provider feeding another call": {Template: `
import "vela/cue"
import "vela/base64"
base: {a: "1"}
merged: cue.#StrategyUnify & {$params: {value: base, patch: {b: "2"}}}
enc: base64.#Encode & {$params: merged.$returns.b}`},

	"two native providers, one nested in the other's output": {Template: `
import "vela/cue"
one: cue.#StrategyUnify & {$params: {value: {x: 1}, patch: {y: 2}}}
two: cue.#StrategyUnify & {$params: {value: one.$returns, patch: {z: 3}}}`},

	// A template names a result once and reads the name, so a call's $params
	// point at an ordinary field rather than at the call behind it. These are
	// the shape real workflow step definitions have.
	"call reading another through a named field": {Template: `
import "vela/base64"
first: base64.#Encode & {$params: "seed"}
named: first.$returns
second: base64.#Encode & {$params: named}`},

	"call reading another through two named fields": {Template: `
import "vela/base64"
first: base64.#Encode & {$params: "seed"}
once: first.$returns
twice: once
second: base64.#Encode & {$params: twice}`},

	"call reading a field of a named struct": {Template: `
import "vela/cue"
import "vela/base64"
merged: cue.#StrategyUnify & {$params: {value: {a: "1"}, patch: {b: "2"}}}
named: merged.$returns
enc: base64.#Encode & {$params: named.b}`},

	"comprehension of calls reading a named field": {Template: `
import "vela/base64"
import "list"
seed: base64.#Encode & {$params: "seed"}
workload: seed.$returns
idx: list.Range(0, 3, 1)
fanout: {
	for i in idx {
		"\(i)": base64.#Encode & {$params: "\(workload)-\(i)"}
	}
}`},

	"call reading a list built from comprehension results": {Template: `
import "vela/base64"
import "list"
seed: base64.#Encode & {$params: "seed"}
workload: seed.$returns
idx: list.Range(0, 3, 1)
fanout: {
	for i in idx {
		"\(i)": base64.#Encode & {$params: "\(workload)-\(i)"}
	}
}
gathered: [for r in fanout {r.$returns}]
final: base64.#Encode & {$params: gathered[0]}`},

	"named field read by a call with no $params of its own": {Template: `
import "vela/base64"
producer: base64.#Encode & {$params: "seed"}
named: producer.$returns
consumer: base64.#Encode & {$params: named}
out: consumer.$returns`},

	// The wait-for-ready shape every KubeVela workflow step definition uses:
	// the dependency is in a comprehension guard, not in $params, and the
	// field $params reads is a disjunction standing on its default until the
	// guard fires.
	"call gated by a comprehension on another call's output": {Template: `
import "vela/base64"
apply: base64.#Encode & {$params: "resource"}
ready: *false | bool
if apply.$returns != "" {
	ready: true
}
gate: base64.#Encode & {$params: "\(ready)"}`},

	"guard reading a nested field of another call's output": {Template: `
import "vela/cue"
import "vela/base64"
apply: cue.#StrategyUnify & {$params: {value: {spec: {}}, patch: {spec: key: "ready"}}}
ready: *false | bool
if apply.$returns.spec != _|_ if apply.$returns.spec.key != "" {
	ready: true
}
gate: base64.#Encode & {$params: "\(ready)"}`},

	"defaulted field never overridden": {Template: `
import "vela/base64"
flag: *"off" | string
a: base64.#Encode & {$params: flag}
b: base64.#Encode & {$params: "independent"}`},

	"unknown provider": {
		Template: `x: {#do: "fn", #provider: "nope"}`,
		WantErr:  "provider nope not found",
	},

	"unknown function": {
		Template: `x: {#do: "nope", #provider: "base64"}`,
		WantErr:  "function nope not found in provider base64",
	},

	"provider returns an error": {
		Template: `
import "vela/base64"
x: base64.#Decode & {$params: "!!!not base64!!!"}`,
		WantErr: "function call error for x",
	},

	"forward reference, consumer declared first": {
		Template: `
import "vela/base64"
a: base64.#Encode & {$params: b.$returns}
b: base64.#Encode & {$params: "x"}`,
		Divergent: "pre-batching the resolver ran calls in walk order and failed " +
			"on a's incomplete $params; batching defers a until b has run",
	},

	"forward reference across nesting": {
		Template: `
import "vela/base64"
first: inner: base64.#Encode & {$params: second.inner.$returns}
second: inner: base64.#Encode & {$params: "x"}`,
		Divergent: "same forward-reference deferral as the flat case",
	},
}

// manifestTemplate is the shape a component definition renders: a large mostly
// static tree with a handful of calls in it. This is the case the resolver is
// actually on the hot path for.
func manifestTemplate(calls, containers int) string {
	var b strings.Builder
	if calls > 0 {
		b.WriteString("import \"vela/base64\"\n")
	}
	for i := 0; i < calls; i++ {
		fmt.Fprintf(&b, "secret%d: base64.#Encode & {$params: \"value-%d\"}\n", i, i)
	}
	b.WriteString(`output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
		name:      "app"
		namespace: "default"
		labels: {"app.oam.dev/name": "app", "app.oam.dev/component": "web"}
		annotations: {"app.oam.dev/revision": "v1"}
	}
	spec: {
		replicas: 3
		selector: matchLabels: "app.oam.dev/component": "web"
		template: {
			metadata: labels: "app.oam.dev/component": "web"
			spec: containers: [
`)
	for i := 0; i < containers; i++ {
		fmt.Fprintf(&b, `			{
				name:  "c%d"
				image: "registry.example.com/img:%d"
				ports: [{containerPort: 8080, protocol: "TCP"}, {containerPort: 8443, protocol: "TCP"}]
				env: [{name: "A", value: "1"}, {name: "B", value: "2"}]
				resources: {limits: {cpu: "1", memory: "1Gi"}, requests: {cpu: "100m", memory: "128Mi"}}
				volumeMounts: [{name: "data", mountPath: "/data"}]
				livenessProbe: httpGet: {path: "/healthz", port: 8080}
			},
`, i, i)
	}
	b.WriteString("			]\n		}\n	}\n}\n")
	return b.String()
}

// widthTemplate is n independent calls, the shape that exposes the quadratic
// behaviour most directly.
func widthTemplate(n int) string {
	var b strings.Builder
	if n == 0 {
		return "placeholder: \"no calls\"\n"
	}
	b.WriteString("import \"vela/base64\"\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "x%d: base64.#Encode & {$params: \"hello-%d\"}\n", i, i)
	}
	return b.String()
}

// depthTemplate is n calls in a single dependency chain, so the resolver can
// only make one call per round. Worst case for batching.
func depthTemplate(n int) string {
	var b strings.Builder
	if n == 0 {
		return "placeholder: \"no calls\"\n"
	}
	b.WriteString("import \"vela/base64\"\nx0: base64.#Encode & {$params: \"seed\"}\n")
	for i := 1; i < n; i++ {
		fmt.Fprintf(&b, "x%d: base64.#Encode & {$params: x%d.$returns}\n", i, i-1)
	}
	return b.String()
}

// mixedTemplate is the shape a real workflow step definition has: a few
// independent calls beside a short chain, over a manifest-sized tree. Neither
// pure width nor pure depth.
func mixedTemplate(independent, chain int) string {
	var b strings.Builder
	b.WriteString("import \"vela/base64\"\n")
	for i := 0; i < independent; i++ {
		fmt.Fprintf(&b, "lookup%d: base64.#Encode & {$params: \"value-%d\"}\n", i, i)
	}
	b.WriteString("c0: base64.#Encode & {$params: \"seed\"}\n")
	for i := 1; i < chain; i++ {
		fmt.Fprintf(&b, "c%d: base64.#Encode & {$params: c%d.$returns}\n", i, i-1)
	}
	b.WriteString(`output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {name: "app", namespace: "default", labels: "app.oam.dev/component": "web"}
	spec: {
		replicas: 3
		selector: matchLabels: "app.oam.dev/component": "web"
		template: spec: containers: [{
			name:  "main"
			image: "registry.example.com/img:1"
			ports: [{containerPort: 8080, protocol: "TCP"}]
			env: [{name: "A", value: "1"}, {name: "B", value: "2"}]
			resources: {limits: {cpu: "1", memory: "1Gi"}, requests: {cpu: "100m", memory: "128Mi"}}
		}]
	}
}
`)
	return b.String()
}

// fanoutTemplate is the apply-in-parallel workflow step definition's shape:
// a short chain with a wide middle. Two calls in sequence, then n that can all
// run together, then one that reads all of them. Each stage reads the one
// before it through a named field, the way a real template does.
func fanoutTemplate(n int) string {
	var b strings.Builder
	b.WriteString(`import "vela/base64"
load: base64.#Encode & {$params: "components"}
loaded: load.$returns
render: base64.#Encode & {$params: loaded}
workload: render.$returns
`)
	b.WriteString("patched: {\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\t\"%d\": base64.#Encode & {$params: \"\\(workload)-%d\"}\n", i, i)
	}
	b.WriteString("}\ngathered: [")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "patched[\"%d\"].$returns, ", i)
	}
	b.WriteString("]\napply: base64.#Encode & {$params: \"\\(len(gathered))\"}\n")
	return b.String()
}

// realWorldTemplate is the shape a workflow step definition actually has, with
// every kind of call in it at once: a few independent lookups, a chain where
// each step reads the one before, a fanout over a loop that all read the same
// thing and nothing of each other, a call that gathers the fanout back up, and
// a manifest-sized tree of plain CUE that has to be walked past all of it.
//
// pkg and def name the provider to call, so the same shape can be driven by a
// pure function or one that waits. attr is put above the fanout, where a
// template would write @concurrency.
func realWorldTemplate(pkg, def string, fanout int, attr string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "import (\n\t\"vela/%s\"\n\t\"list\"\n)\n", pkg)

	b.WriteString("\n// independent: read what the step was configured with\n")
	for _, name := range []string{"image", "registry", "secretRef"} {
		fmt.Fprintf(&b, "cfg_%s: %s.%s & {$params: \"%s\"}\n", name, pkg, def, name)
	}

	b.WriteString("\n// a chain: load, then render what was loaded\n")
	fmt.Fprintf(&b, "load: %s.%s & {$params: \"components\"}\n", pkg, def)
	b.WriteString("loaded: load.$returns\n")
	fmt.Fprintf(&b, "render: %s.%s & {$params: loaded}\n", pkg, def)
	b.WriteString("workload: render.$returns\n")

	b.WriteString("\n// fanout: one per replica, all reading the render, none reading each other\n")
	fmt.Fprintf(&b, "idx: list.Range(0, %d, 1)\n", fanout)
	b.WriteString("patched: {\n\tfor i in idx {\n")
	fmt.Fprintf(&b, "\t\t\"\\(i)\": %s.%s & {$params: \"\\(workload)-\\(i)\"}\n", pkg, def)
	b.WriteString("\t}\n}")
	b.WriteString(attr)
	b.WriteString("\n")

	b.WriteString("\n// gather the fanout back up, then one last call over it\n")
	b.WriteString("gathered: [for p in patched {p.$returns}]\n")
	fmt.Fprintf(&b, "summary: %s.%s & {$params: \"\\(len(gathered))\"}\n", pkg, def)

	b.WriteString(`
// and the manifest the step renders, which every walk goes through
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
		name:      "app"
		namespace: "default"
		labels: {"app.oam.dev/name": "app", "app.oam.dev/component": "web"}
		annotations: {"app.oam.dev/revision": "v1"}
	}
	spec: {
		replicas: 3
		selector: matchLabels: "app.oam.dev/component": "web"
		template: {
			metadata: labels: "app.oam.dev/component": "web"
			spec: containers: [
`)
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&b, `				{
					name:  "c%d"
					image: "registry.example.com/img:%d"
					ports: [{containerPort: 8080, protocol: "TCP"}, {containerPort: 8443, protocol: "TCP"}]
					env: [{name: "A", value: "1"}, {name: "B", value: "2"}]
					resources: {limits: {cpu: "1", memory: "1Gi"}, requests: {cpu: "100m", memory: "128Mi"}}
					volumeMounts: [{name: "data", mountPath: "/data"}]
					livenessProbe: httpGet: {path: "/healthz", port: 8080}
				},
`, i, i)
	}
	b.WriteString("			]\n		}\n	}\n}\n")
	return b.String()
}

// namespaceFanTemplate is the multi-namespace shape: read n namespaces, then
// create a resource in each, with each create reading its own namespace's
// result. Two wide stages one after the other, rather than one fanout - every
// read can go at once, every create can go at once, and no create can go
// before its own read.
//
// attr is put above both stages, where a template would write @concurrency.
func namespaceFanTemplate(n int, attr string) string {
	var b strings.Builder
	b.WriteString("import (\n\t\"vela/io\"\n\t\"list\"\n)\n")

	b.WriteString("\n// independent: what the step was configured with\n")
	for _, name := range []string{"clusterEndpoint", "imageRegistry", "pullSecret", "owner"} {
		fmt.Fprintf(&b, "cfg_%s: io.#Get & {$params: \"%s\"}\n", name, name)
	}

	fmt.Fprintf(&b, "\nnamespaces: list.Range(0, %d, 1)\n", n)

	b.WriteString("\n// stage one: read every namespace\n")
	b.WriteString("reads: {\n\tfor i in namespaces {\n")
	b.WriteString("\t\t\"\\(i)\": io.#Get & {$params: \"namespace-\\(i)\"}\n")
	b.WriteString("\t}\n}")
	b.WriteString(attr)
	b.WriteString("\n")

	b.WriteString("\n// stage two: create a resource in each, reading its own namespace\n")
	b.WriteString("creates: {\n\tfor i in namespaces {\n")
	b.WriteString("\t\t\"\\(i)\": io.#Apply & {$params: \"\\(reads[\"\\(i)\"].$returns)/config\"}\n")
	b.WriteString("\t}\n}")
	b.WriteString(attr)
	b.WriteString("\n")

	b.WriteString("\nsummary: io.#Get & {$params: \"\\(len(namespaces))\"}\n")
	b.WriteString(`
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {name: "app", namespace: "default", labels: "app.oam.dev/component": "web"}
	spec: template: spec: containers: [{
		name:  "main"
		image: "registry.example.com/img:1"
		ports: [{containerPort: 8080, protocol: "TCP"}]
		resources: limits: {cpu: "1", memory: "1Gi"}
	}]
}
`)
	return b.String()
}

// deepFanTemplate is the shape a step working over a fleet has: read every
// namespace, read a config map in each, then create a resource from what came
// back. Three wide stages one behind the other - 3n calls, where everything
// within a stage can go at once and nothing can go before the stage above it.
func deepFanTemplate(n int, attr string) string {
	var b strings.Builder
	b.WriteString("import (\n\t\"vela/io\"\n\t\"list\"\n)\n")

	b.WriteString("\n// independent: what the step was configured with\n")
	for _, name := range []string{"clusterEndpoint", "imageRegistry", "pullSecret", "owner"} {
		fmt.Fprintf(&b, "cfg_%s: io.#Get & {$params: \"%s\"}\n", name, name)
	}

	fmt.Fprintf(&b, "\nnamespaces: list.Range(0, %d, 1)\n", n)

	for _, stage := range []struct{ name, fn, params string }{
		{"nsReads", "#Get", `"namespace-\(i)"`},
		{"cmReads", "#Get", `"\(nsReads["\(i)"].$returns)/config"`},
		{"creates", "#Apply", `"\(cmReads["\(i)"].$returns)/resource"`},
	} {
		fmt.Fprintf(&b, "\n%s: {\n\tfor i in namespaces {\n", stage.name)
		fmt.Fprintf(&b, "\t\t\"\\(i)\": io.%s & {$params: %s}\n", stage.fn, stage.params)
		b.WriteString("\t}\n}")
		b.WriteString(attr)
		b.WriteString("\n")
	}

	b.WriteString("\nsummary: io.#Get & {$params: \"\\(len(namespaces))\"}\n")
	return b.String()
}
