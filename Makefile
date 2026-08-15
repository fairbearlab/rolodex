.PHONY: all test lint fmt vet vuln build ci

all: build

test:
	go test -race ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .

vet:
	go vet ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

build:
	go build ./...

# Everything CI runs, runnable locally before pushing
ci: vet lint test vuln build
