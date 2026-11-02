# Contributing

`make ci` is the merge gate: it runs vet, lint, the version check, tests, govulncheck, coverage and a
build, the same as CI. Run it before opening a pull request.

## bin/env-check (optional)

`bin/env-check` checks a checkout before you build or test in it: that the Go and golangci-lint pins in
`go.mod`, `.tool-versions` and the workflows agree, that your local tools match them, that the files the
tests need are present, and that you are working in a git worktree rather than the main checkout. It is a
thin wrapper around the shared `loops env-check` engine (the `loops` plugin in
[agentic-tools](https://github.com/adambware/agentic-tools)), configured in `.loops/env-check.toml`.

- Exit 0: no FAIL. Exit 1: at least one FAIL; every FAIL and WARN line ends with a fix.
- **Exit 3 (`env-check: not run`) means the engine is not installed. You can ignore it**; `make ci`
  stays the gate.
- `bin/env-check --static` reads repo files only. `ENV_CHECK_ALLOW_MAIN=1` allows a deliberate run in
  the main checkout.

CI's golangci-lint version comes from the shared reusable workflow, not `.tool-versions`. `make vuln`
runs `govulncheck@latest` by design and needs network access.
