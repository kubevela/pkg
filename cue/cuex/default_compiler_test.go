/*
Copyright 2023 The KubeVela Authors.

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
	"context"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/util/stringtools"
)

func TestDefaultCompiler(t *testing.T) {
	compiler := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	val, err := compiler.CompileString(ctx, `
		import (
			"vela/cue"
			"strings"
		)

		scaler: {
	        description: "desc for scaler trait"
	        labels: {}
	        type: "trait"
		}
		
		_enc: cue.Encode & {
			$params: {
		        parameter: {
	                // +usage=Specify the number of workload
	                replicas: *1 | int
		        }
		        // +patchStrategy=retainKeys
		        patch: spec: replicas: parameter.replicas
			}
		}

		_template: _enc.$returns

		#EncodeDefinition: {
			$params: {
				name: string
				meta: {...}
				template: string
			}

			$returns: {
				apiVersion: "core.oam.dev/v1beta1"
				kind: strings.ToTitle($params.meta.type) + "Definition"
				metadata: name: $params.name
				metadata: namespace: "vela-system"
				metadata: annotations: "definition.oam.dev/description": $params.meta.description
				spec: schematic: cue: template: $params.template
			}
		}

		_encode: #EncodeDefinition & {
			$params: {
				name: "scaler"
				meta: scaler
				template: _template
			}
		}

		$returns: _encode.$returns
	`)
	require.NoError(t, err)
	bs, err := val.LookupPath(cue.ParsePath("$returns")).MarshalJSON()
	require.NoError(t, err)
	bs, err = yaml.JSONToYAML(bs)
	require.NoError(t, err)
	require.Equal(t, stringtools.TrimLeadingIndent(`
		apiVersion: core.oam.dev/v1beta1
		kind: TraitDefinition
		metadata:
		  annotations:
		    definition.oam.dev/description: desc for scaler trait
		  name: scaler
		  namespace: vela-system
		spec:
		  schematic:
		    cue:
		      template: "parameter: {\n\t// +usage=Specify the number of workload\n\treplicas:
		        *1 | int\n}\n// +patchStrategy=retainKeys\npatch: spec: replicas: parameter.replicas\n"
	`), string(bs))
}
