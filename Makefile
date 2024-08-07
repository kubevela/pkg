generate:
	go generate ./apis/...

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

unit-test:
	go test -v -coverpkg=./... -coverprofile=coverage.txt ./...

lint:
	golangci-lint run ./...

reviewable: generate fmt vet tidy lint
