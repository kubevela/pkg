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

package cuex

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/util"
)

func TestCueXExample(t *testing.T) {
	compiler := NewCompilerWithDefaultInternalPackages()

	src := `
		import (
			"vela/http"
			"vela/kube"
		)

		getIP: http.#Get & {
			$params: url: "https://api.ipify.org/"
		}

		localIP: getIP.$returns.body

		apply: kube.#Apply & {
			$params: resource: {
				apiVersion: "v1"
				kind: "Secret"
				metadata: {
					name: "ip"
					namespace: "default"
				}
				stringData: ip: localIP
			}
		}
	`

	val, err := compiler.CompileString(context.Background(), src)
	require.NoError(t, err)
	s, err := util.ToString(val)
	require.NoError(t, err)
	fmt.Println(s)
}
