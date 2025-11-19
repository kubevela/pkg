package context

import (
	"context"
	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex/interfaces"
)

type CompilerKey struct{}
type CueContextKey struct{}

func WithCueContext(ctx context.Context, cueCtx *cue.Context) context.Context {
	return context.WithValue(ctx, CueContextKey{}, cueCtx)
}

func GetCueContext(ctx context.Context) *cue.Context {
	if c, ok := ctx.Value(CueContextKey{}).(*cue.Context); ok {
		return c
	}
	return nil
}

func WithCompiler(ctx context.Context, c interfaces.CueXCompiler) context.Context {
	return context.WithValue(ctx, CompilerKey{}, c)
}

func GetCompiler(ctx context.Context) interfaces.CueXCompiler {
	if c, ok := ctx.Value(CompilerKey{}).(interfaces.CueXCompiler); ok {
		return c
	}
	return nil
}
