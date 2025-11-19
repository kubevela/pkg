package types

import "context"

// CompileConfig config for running compile process
type CompileConfig struct {
	ResolveProviderFunctions bool
	PreResolveMutators       []func(context.Context, string) (string, error)
}
