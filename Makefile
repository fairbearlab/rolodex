.PHONY: all test lint fmt vet vuln build cover ci

# Floor set from the measured 61.8% at the time this gate landed (session 4), minus a
# point of headroom. It catches regression; it is not a stretch goal. internal/review
# (27.4%) is the known drag — raise this as that package gets tests.
COVER_MIN ?= 99

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

cover:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	@total=$$(go tool cover -func=coverage.out | awk '/^total:/ {sub("%","",$$3); print $$3}'); \
	echo "total coverage: $$total% (min $(COVER_MIN)%)"; \
	awk -v t=$$total -v m=$(COVER_MIN) 'BEGIN { exit (t+0 < m+0) ? 1 : 0 }'

# Everything CI runs, runnable locally before pushing
ci: vet lint test vuln cover build
