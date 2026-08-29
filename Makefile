.PHONY: all test lint fmt vet vuln build cover version-check ci

# Floor set from the measured 61.8% at the time this gate landed (session 4), minus a
# point of headroom. It catches regression; it is not a stretch goal. internal/review
# (27.4%) is the known drag — raise this as that package gets tests.
COVER_MIN ?= 60

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

# cmd/rolodex/version.txt is //go:embed-ed into the binary, so it is a second
# source of truth that VERSION cannot update on its own. It drifted once and
# shipped a 0.4.0 release that reported itself as 0.3.0.
version-check:
	@a=$$(tr -d ' \t\n\r' < VERSION); b=$$(tr -d ' \t\n\r' < cmd/rolodex/version.txt); \
	if [ "$$a" != "$$b" ]; then \
		echo "VERSION is $$a but cmd/rolodex/version.txt is $$b; the built binary would report the wrong version"; \
		exit 1; \
	fi; \
	echo "version: $$a (VERSION and embedded copy agree)"

# Everything CI runs, runnable locally before pushing
ci: vet lint version-check test vuln cover build
