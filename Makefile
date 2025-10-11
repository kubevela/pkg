include makefiles/const.mk

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)
ENVTEST ?= $(LOCALBIN)/setup-envtest
ENVTEST_K8S_VERSION = 1.30.0

generate:
	go generate ./apis/...

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

unit-test: envtest
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -v -coverpkg=./... -coverprofile=coverage.txt ./...

lint:
	golangci-lint run ./...

reviewable: generate fmt vet tidy lint

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# check-diff: Execute auto-gen code commands and ensure branch is clean.
check-diff: reviewable
	git --no-pager diff
	git diff --quiet || ($(ERR) please run 'make reviewable' to include all changes && false)
	@$(OK) branch is clean