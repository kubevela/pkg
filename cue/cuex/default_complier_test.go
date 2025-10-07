package cuex_test

import (
	"context"

	"cuelang.org/go/cue"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/cue/cuex/model/sets"
	"github.com/kubevela/pkg/util/stringtools"
)

var _ = Describe("Test Default Compiler", func() {
	ctx := context.Background()
	compiler := cuex.NewCompilerWithDefaultInternalPackages()

	It("test vela/cue internal packages", func() {
		val, err := compiler.CompileString(ctx, `
		import (
			"vela/cue"
		)
		secret: {
			apiVersion: "v1"
			kind:       "Secret"
			metadata: {
				name:      "ip"
				namespace: "default"
			}
		}
		patch: cue.#StrategyUnify & {
			$params: {
				value: secret
				patch: {
					stringData: ip: "127.0.0.1"
				}
			}
		}
`)
		Expect(err).Should(BeNil())
		ret := val.LookupPath(cue.ParsePath("patch.$returns"))
		retStr, err := sets.ToString(ret)
		Expect(err).Should(BeNil())

		Expect(stringtools.TrimLeadingIndent(retStr)).Should(BeEquivalentTo(stringtools.TrimLeadingIndent(`
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      "ip"
			namespace: "default"
		}
		stringData: {
			ip: "127.0.0.1"
		}
`)))
	})
})
