GLOBAL_GOLANGCILINT := $(shell which golangci-lint)
GOBIN_GOLANGCILINT:= $(shell which $(GOBIN)/golangci-lint)

golangci:
ifeq ($(shell $(GLOBAL_GOLANGCILINT) version --format short), $(GOLANGCILINT_VERSION))
	@$(OK) golangci-lint is already installed
GOLANGCILINT=$(GLOBAL_GOLANGCILINT)
else ifeq ($(shell $(GOBIN_GOLANGCILINT) version --format short), $(GOLANGCILINT_VERSION))
	@$(OK) golangci-lint is already installed
GOLANGCILINT=$(GOBIN_GOLANGCILINT)
else
	@{ \
	set -e ;\
	echo 'installing golangci-lint-$(GOLANGCILINT_VERSION)' ;\
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) v$(GOLANGCILINT_VERSION) ;\
	echo 'Successfully installed' ;\
	}
GOLANGCILINT=$(GOBIN)/golangci-lint
endif