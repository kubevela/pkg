package interfaces

import (
	"context"
	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex/types"
)

type CueXCompiler interface {
	CompileString(ctx context.Context, src string) (cue.Value, error)
	CompileStringWithOptions(ctx context.Context, src string, opts ...CompileOption) (cue.Value, error)
}

// CompileOption options for compile cue string
type CompileOption interface {
	ApplyTo(config *types.CompileConfig)
}
