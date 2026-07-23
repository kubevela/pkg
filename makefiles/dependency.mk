GLOBAL_GOLANGCILINT := $(shell which golangci-lint 2>/dev/null)
GOBIN_GOLANGCILINT:= $(shell which $(GOBIN)/golangci-lint 2>/dev/null)

golangci:
ifeq ($(and $(GLOBAL_GOLANGCILINT),$(shell $(GLOBAL_GOLANGCILINT) version --short)), $(GOLANGCILINT_VERSION))
	@$(OK) golangci-lint is already installed
GOLANGCILINT=$(GLOBAL_GOLANGCILINT)
else ifeq ($(and $(GOBIN_GOLANGCILINT),$(shell $(GOBIN_GOLANGCILINT) version --short)), $(GOLANGCILINT_VERSION))
	@$(OK) golangci-lint is already installed
GOLANGCILINT=$(GOBIN_GOLANGCILINT)
else
	@{ \
	set -e ;\
	echo 'installing golangci-lint-$(GOLANGCILINT_VERSION)' ;\
	GOBIN=$(GOBIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCILINT_VERSION) ;\
	echo 'Successfully installed' ;\
	}
GOLANGCILINT=$(GOBIN)/golangci-lint
endif