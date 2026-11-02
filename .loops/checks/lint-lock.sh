#!/bin/sh
# d.lint-lock: golangci-lint takes a per-user lock, so `make lint` in two worktrees at once can fail.
if grep -Eqs 'allow-(serial|parallel)-runners' .golangci.yml Makefile; then
  echo "PASS d.lint-lock golangci-lint runner lock is configured"
else
  echo "INFO d.lint-lock golangci-lint takes a per-user lock; run make lint in one worktree at a time"
fi
