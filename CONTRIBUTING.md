# Contributing

Run `make ci` before opening a pull request. It runs the local merge checks: vet, lint, the version
check, race tests, govulncheck, the coverage floor and a build. CI runs these plus a `go mod tidy` check,
a benchmark smoke run and CodeQL.

## bin/env-check (optional)

`bin/env-check` checks a checkout before you build or test in it: that the Go and golangci-lint pins in
`go.mod`, `.tool-versions` and the workflows agree, that your local tools match them, that the files the
tests need are present, and that you are working in a git worktree rather than the main checkout. It is a
thin wrapper around the shared `loops env-check` engine (the `loops` plugin in
[agentic-tools](https://github.com/adambware/agentic-tools)), configured in `.loops/env-check.toml`.

- Exit 0: no FAIL. Exit 1: at least one FAIL; every FAIL and WARN line ends with a fix. Exit 2: a usage
  or config error, for example an engine older than the config needs; update the engine.
- **Exit 3 (`env-check: not run`) means no engine is installed. You can ignore it**;
  `make ci` stays the gate. If the engine is installed but node cannot run, it exits 1 with a
  `FAIL d.tools` row instead.
- `bin/env-check --static` reads repo files only. `ENV_CHECK_ALLOW_MAIN=1` allows a deliberate run in
  the main checkout.

CI's golangci-lint version comes from the shared reusable workflow, not `.tool-versions`. `make vuln`
runs `govulncheck@latest` by design and needs network access.
