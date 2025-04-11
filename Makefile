BINARY_NAME=terraform-provider-hostinger

.PHONY: build install test docs fmt vet

build:
	go build -o $(BINARY_NAME)

install:
	go install .

test:
	go test -v ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

release:
	goreleaser release --clean

