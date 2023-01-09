generate:
	go generate ./apis/...

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

unit-test:
	go test -v -coverpkg=./... -coverprofile=/tmp/vela-pkg-coverage.txt ./...

reviewable: fmt vet
