#!/usr/bin/env bash
# Fixture suite for bin/env-check. Every case builds a throwaway repo under a
# temporary HOME, with stub go/golangci-lint/make on PATH, and asserts on the
# exit code and the status line plus its reason text.
# Bash 3.2 and bash 5 compatible.

set -u
unset CI GITHUB_ACTIONS ENV_CHECK_ALLOW_MAIN GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR GIT_INDEX_FILE GOTOOLCHAIN GOFLAGS COMPOSE_PROJECT_NAME
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null

SELF=$0
case $SELF in
/*) ;;
*) SELF=$PWD/$SELF ;;
esac
REPO=$(cd "$(dirname "$SELF")/.." && pwd -P)
CHECK=$REPO/bin/env-check

HOME=$(mktemp -d) || exit 1
export HOME
HOME=$(cd "$HOME" && pwd -P)
export HOME
export GIT_CEILING_DIRECTORIES
GIT_CEILING_DIRECTORIES=$(dirname "$HOME")
trap 'rm -rf "$HOME"' EXIT

STUBS=$HOME/stubs
mkdir -p "$STUBS"
cat >"$STUBS/go" <<'STUB'
#!/bin/sh
printf '%s\n' "${GOTOOLCHAIN:-unset}" >>"$STUB_LOG"
printf '%s\n' "$*" >>"$STUB_ARGV_LOG"
case $1 in
version)
	if [ "${STUB_GO_FAIL:-0}" = 1 ]; then
		echo "go: broken stub" >&2
		exit 1
	fi
	echo "go version go${STUB_GO} stub/stub"
	;;
env)
	case $2 in
	GOBIN) echo "${STUB_GOBIN:-}" ;;
	GOROOT) echo "${STUB_GOROOT:-}" ;;
	GOPATH) echo "${STUB_GOPATH:-}" ;;
	GOWORK) echo "${STUB_GOWORK:-}" ;;
	*) echo "go${STUB_GO}" ;;
	esac
	;;
list)
	[ -z "${STUB_GO_LIST_MSG:-}" ] || echo "$STUB_GO_LIST_MSG" >&2
	exit "${STUB_GO_LIST_RC:-0}"
	;;
esac
exit 0
STUB
# git wrapper: log argv, then run the real git.
cat >"$STUBS/git" <<STUB
#!/bin/sh
printf '%s\n' "\$*" >>"\$STUB_GIT_LOG"
exec "$(command -v git)" "\$@"
STUB
cat >"$STUBS/golangci-lint" <<'STUB'
#!/bin/sh
if [ "${STUB_LINT_FAIL:-0}" = 1 ]; then
	echo "$HOME/.asdf/shims/golangci-lint: No version is set for command golangci-lint" >&2
	exit 1
fi
if [ "${STUB_LINT_DEVEL:-0}" = 1 ]; then
	echo "golangci-lint (devel) built with go1.99.0 from x on y"
	exit 0
fi
echo "golangci-lint has version ${STUB_LINT} built with go1.99.0 from x on y"
STUB
cat >"$STUBS/make" <<'STUB'
#!/bin/sh
exit 0
STUB
chmod +x "$STUBS/go" "$STUBS/git" "$STUBS/golangci-lint" "$STUBS/make"

export STUB_LOG=$HOME/go-calls.log
: >"$STUB_LOG"
export STUB_ARGV_LOG=$HOME/go-argv.log STUB_GIT_LOG=$HOME/git-argv.log
: >"$STUB_ARGV_LOG"
: >"$STUB_GIT_LOG"

GIT="git -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false"
N=0
OUT=
RC=0
ALL=$HOME/all-output.log
: >"$ALL"

fail() {
	echo "FAIL [$CASE]: $1" >&2
	echo "--- output (rc=$RC) ---" >&2
	printf '%s\n' "$OUT" >&2
	exit 1
}

# mkfixture NAME: create a main checkout fixture at $HOME/NAME, committed.
mkfixture() {
	local d=$HOME/$1
	mkdir -p "$d/cmd/rolodex" "$d/testdata" "$d/.github/workflows"
	(
		cd "$d" || exit 1
		git -c init.defaultBranch=main init -q
		cat >go.mod <<'F'
module example.invalid/fixture

go 9.8.7
F
		echo "h1:fixture" >go.sum
		echo "0.0.1" >VERSION
		echo "0.0.1" >cmd/rolodex/version.txt
		cat >.golangci.yml <<'F'
version: "4"
F
		cat >.tool-versions <<'F'
golang 9.8.7
golangci-lint 4.5.6
F
		cat >.github/workflows/ci.yml <<'F'
name: CI
jobs:
  cover:
    steps:
      - uses: example/setup@v1
        with:
          go-version-file: go.mod
F
		cat >Makefile <<'F'
lint:
	golangci-lint run
F
		echo "BEGIN:VCARD" >testdata/google.vcf
		echo "BEGIN:VCARD" >testdata/icloud.vcf
		git add -A
		$GIT commit -q -m init
	) || exit 1
}

# mkworktree NAME: add a linked worktree $HOME/NAME-wt for fixture NAME.
mkworktree() {
	git -C "$HOME/$1" worktree add -q -b wt "$HOME/$1-wt" HEAD || exit 1
}

# run_in DIR [VAR=value...]: run env-check with cwd DIR; sets OUT and RC.
run_in() {
	local dir=$1
	shift
	OUT=$(cd "$dir" && env PATH="$STUBS:/usr/bin:/bin" "$@" "$CHECK" 2>&1)
	RC=$?
	printf '%s\n' "$OUT" >>"$ALL"
}

# Defaults for stubs (reset per case).
defaults() {
	export STUB_GO=9.8.7 STUB_LINT=4.5.6 STUB_GO_LIST_RC=0 STUB_GO_LIST_MSG='' STUB_GO_FAIL=0 STUB_LINT_FAIL=0 STUB_LINT_DEVEL=0
	export STUB_GOBIN='' STUB_GOROOT='' STUB_GOPATH='' STUB_GOWORK=''
}

begin_case() {
	CASE=$1
	N=$((N + 1))
	defaults
}

want_rc() { [ "$RC" -eq "$1" ] || fail "expected exit $1"; }
want() { printf '%s\n' "$OUT" | grep -qE "$1" || fail "missing /$1/"; }
want_not() { ! printf '%s\n' "$OUT" | grep -qE "$1" || fail "unexpected /$1/"; }
want_not_f() { ! printf '%s\n' "$OUT" | grep -qF "$1" || fail "unexpected literal $1"; }

# Fresh fixture + linked worktree per mutation case.
fresh() {
	rm -rf "$HOME/fx" "$HOME/fx-wt"
	mkfixture fx
	mkworktree fx
}

# commit_all DIR: commit pending changes in the worktree.
commit_all() {
	(cd "$1" && git add -A && $GIT commit -q --allow-empty -m change)
}

WT=$HOME/fx-wt

# 1. healthy worktree
begin_case "healthy worktree"
fresh
run_in "$WT"
want_rc 0
want '^a PASS '
want '^b SKIP '
want '^c PASS '
want '^d PASS '
want '^e SKIP '
want '^env-check: 0 fail, 0 warn$'
want 'info: CI lint version'
HEALTHY=$OUT

# 3-5. main checkout
begin_case "main checkout fails"
run_in "$HOME/fx"
want_rc 1
want '^c FAIL .*shared \(main\) checkout'

begin_case "main checkout allowed"
run_in "$HOME/fx" ENV_CHECK_ALLOW_MAIN=1
want_rc 0
want '^c WARN '

begin_case "main checkout ignores CI env"
run_in "$HOME/fx" CI=true GITHUB_ACTIONS=true
want_rc 1
want '^c FAIL '

# 6. subdirectory
begin_case "subdirectory run identical"
run_in "$WT/cmd/rolodex"
[ "$OUT" = "$HEALTHY" ] || fail "output differs from root run"

# 7-9. installed go
begin_case "go patch higher"
export STUB_GO=9.8.9
run_in "$WT"
want_rc 0
want '^a WARN .*patch differs: local 9.8.9, CI 9.8.7'

begin_case "go lower"
export STUB_GO=9.8.6
run_in "$WT"
want_rc 1
want '^a FAIL .*local go 9.8.6 < go.mod 9.8.7'
want 'info: module cache not checked'
want '^d PASS tools and files ok \(module cache not checked\)'

begin_case "go minor differs"
export STUB_GO=9.9.0
run_in "$WT"
want_rc 1
want '^a FAIL '

# 10-12. tracked pins
begin_case "tool-versions disagrees"
fresh
printf 'golang 9.8.6\ngolangci-lint 4.5.6\n' >"$WT/.tool-versions"
run_in "$WT"
want_rc 1
want '^a FAIL .*tracked pins disagree'

begin_case "tool-versions golang missing"
fresh
printf 'golangci-lint 4.5.6\n' >"$WT/.tool-versions"
run_in "$WT"
want_rc 1
want '^a FAIL .*could not parse golang from .tool-versions'

begin_case "go.mod without patch"
fresh
printf 'module example.invalid/fixture\n\ngo 9.8\n' >"$WT/go.mod"
run_in "$WT"
want_rc 0
want '^a WARN .*go.mod has no patch'

# 13. workflow without go-version-file
begin_case "no workflow go-version-file"
fresh
printf 'name: CI\njobs: {}\n' >"$WT/.github/workflows/ci.yml"
run_in "$WT"
want_rc 0
want '^a PASS '
want 'info: no local go-version-file'

# 13b. workflow literal that disagrees
begin_case "workflow literal disagrees"
fresh
printf 'name: CI\njobs:\n  x:\n    steps:\n      - with:\n          go-version: "9.7.1" # old\n' >"$WT/.github/workflows/ci.yml"
run_in "$WT"
want_rc 1
want '^a FAIL .*tracked pins disagree'

# 14-16. golangci-lint
begin_case "lint major differs"
fresh
export STUB_LINT=5.0.0
run_in "$WT"
want_rc 1
want '^a FAIL '

begin_case "lint differs from tool-versions"
fresh
export STUB_LINT=4.5.7
run_in "$WT"
want_rc 0
want '^a WARN .*lint differs'

begin_case "lint devel build"
fresh
export STUB_LINT_DEVEL=1
run_in "$WT"
want_rc 0
want_not '^a PASS '
want_not 'golangci-lint 1\.99\.0'
want '^a WARN .*could not parse golangci-lint version'

# 17. lint not runnable
begin_case "lint not runnable"
fresh
export STUB_LINT_FAIL=1
run_in "$WT"
want_rc 1
want '^d FAIL .*No version is set'
want '^d FAIL .*run asdf install'
# Redaction: the stub error embeds the (fixture) HOME; it must print as ~.
want '^d FAIL .*~/\.asdf/shims/golangci-lint'
want_not_f "$HOME"

# 18. missing version.txt
begin_case "version.txt missing"
fresh
rm "$WT/cmd/rolodex/version.txt"
run_in "$WT"
want_rc 1
want '^d FAIL .*version.txt'

# 19. cold module cache
begin_case "cold module cache"
fresh
export STUB_GO_LIST_RC=1 STUB_GO_LIST_MSG="x: cannot find module providing package x: module lookup disabled by GOPROXY=off"
run_in "$WT"
want_rc 0
want '^d WARN .*module cache cold'

begin_case "go list fails for another reason"
fresh
export STUB_GO_LIST_RC=1 STUB_GO_LIST_MSG="internal/zz/zz.go:3:8: package example.invalid/nope is not in std" STUB_GOWORK=/some/go.work
run_in "$WT"
want_rc 0
want '^d WARN go list failed: internal/zz/zz.go.*\(go.work: /some/go.work\)'
want_not 'module cache cold'

# 20. rot guard
rot_case() {
	# $1 label, $2 committed|untracked, $3 path, $4 content
	begin_case "rot guard: $1 ($2)"
	fresh
	mkdir -p "$(dirname "$WT/$3")"
	printf '%s\n' "$4" >"$WT/$3"
	if [ "$2" = committed ]; then
		commit_all "$WT"
	fi
	run_in "$WT"
	want_rc 1
	want '^b FAIL .*container files found'
	want '^e FAIL .*container files found'
}
for mode in committed untracked; do
	rot_case compose.yml "$mode" compose.yml "services: {}"
	rot_case compose.override.yml "$mode" compose.override.yml "services: {}"
	rot_case nested-compose.yaml "$mode" deploy/x/y/compose.yaml "services: {}"
	rot_case app.Dockerfile "$mode" app.Dockerfile "FROM scratch"
	rot_case Containerfile "$mode" Containerfile "FROM scratch"
done
begin_case "rot guard: Makefile docker compose"
fresh
printf 'up:\n\tdocker compose up\n' >>"$WT/Makefile"
run_in "$WT"
want_rc 1
want '^b FAIL .*Makefile'
want '^e FAIL '

# 20b. rot guard: real container command use in workflows and Makefile
rot_content_case() {
	# $1 label, $2 path, $3 content
	begin_case "rot guard: $1"
	fresh
	mkdir -p "$(dirname "$WT/$2")"
	printf '%s\n' "$3" >"$WT/$2"
	run_in "$WT"
	want_rc 1
	want '^b FAIL .*container files found'
	want '^e FAIL .*container files found'
	want "$2"
}
rot_content_case "workflow docker run" .github/workflows/ci.yml 'jobs: {x: {steps: [{run: docker run busybox}]}}'
rot_content_case "workflow .yaml docker build" .github/workflows/x.yaml 'run: docker build .'
rot_content_case "workflow build-push-action" .github/workflows/ci.yml '      - uses: docker/build-push-action@v6'

# 20c. rot guard must not trip on documentation or unrelated mentions
nonrot_case() {
	# $1 label, $2 path, $3 content
	begin_case "no rot: $1"
	fresh
	mkdir -p "$(dirname "$WT/$2")"
	printf '%s\n' "$3" >"$WT/$2"
	run_in "$WT"
	want_rc 0
	want '^b SKIP '
	want '^e SKIP '
}
nonrot_case "Dockerfile-notes.md" docs/Dockerfile-notes.md "notes"
nonrot_case "Dockerfile.md" docs/Dockerfile.md "notes"
nonrot_case "setup-buildx only" .github/workflows/x.yml '      - uses: docker/setup-buildx-action@v3'
nonrot_case "Makefile comment" Makefile '# docker compose up is not used here'

# 20d. toolchain directive, workflow expressions and go-version forms
begin_case "go.mod toolchain is the CI pin"
fresh
printf 'module example.invalid/fixture\n\ngo 9.8.7\n\ntoolchain go9.8.8\n' >"$WT/go.mod"
printf 'golang 9.8.8\ngolangci-lint 4.5.6\n' >"$WT/.tool-versions"
export STUB_GO=9.8.8
run_in "$WT"
want_rc 0
want '^a PASS '
want 'CI Go pin 9.8.8 \(go.mod toolchain\)'

begin_case "go.mod toolchain vs tool-versions"
fresh
printf 'module example.invalid/fixture\n\ngo 9.8.7\n\ntoolchain go9.8.8\n' >"$WT/go.mod"
export STUB_GO=9.8.8
run_in "$WT"
want_rc 1
want '^a FAIL .*tracked pins disagree: \.tool-versions 9\.8\.7, go\.mod 9\.8\.8'

begin_case "workflow expression is info only"
fresh
# shellcheck disable=SC2016
printf 'name: CI\njobs:\n  x:\n    steps:\n      - with:\n          go-version: ${{ matrix.go }}\n' >"$WT/.github/workflows/ci.yml"
run_in "$WT"
want_rc 0
want '^a PASS '
want 'info: unresolvable expression in \.github/workflows/ci\.yml'

begin_case "workflow go-version-file other than go.mod"
fresh
printf 'name: CI\njobs:\n  x:\n    steps:\n      - with:\n          go-version-file: sub/go.mod\n' >"$WT/.github/workflows/ci.yml"
run_in "$WT"
want_rc 0
want '^a WARN .*reads sub/go\.mod'

wf_version_case() {
	# $1 go-version value, $2 expected exit, $3 status line regex
	begin_case "workflow go-version $1"
	fresh
	printf 'name: CI\njobs:\n  x:\n    steps:\n      - with:\n          go-version: %s\n' "$1" >"$WT/.github/workflows/ci.yml"
	run_in "$WT"
	want_rc "$2"
	want "$3"
}
wf_version_case "'9.8.x'" 0 '^a WARN .*no patch'
wf_version_case "'9.7.x'" 1 '^a FAIL .*tracked pins disagree'
wf_version_case "stable" 0 "^a WARN .*go-version 'stable' is not a plain version"
wf_version_case "['9.8.7']" 0 '^a WARN .*not a plain version'

begin_case "tool-versions golang without patch"
fresh
printf 'golang 9.8\ngolangci-lint 4.5.6\n' >"$WT/.tool-versions"
run_in "$WT"
want_rc 0
want '^a WARN .*\.tool-versions has no patch'

begin_case "tool-versions golangci-lint missing"
fresh
printf 'golang 9.8.7\n' >"$WT/.tool-versions"
run_in "$WT"
want_rc 1
want '^a FAIL .*could not parse golangci-lint from \.tool-versions'

begin_case "go not runnable"
fresh
export STUB_GO_FAIL=1
run_in "$WT"
want_rc 1
want '^a FAIL .*go not runnable'
want '^d FAIL .*go not runnable: go: broken stub'
want_not '^d FAIL .*run asdf install'
want 'info: module cache not checked'

# 20e. several golangci-lint binaries can shadow each other
begin_case "second golangci-lint binary"
fresh
mkdir -p "$HOME/gobin"
cp "$STUBS/golangci-lint" "$HOME/gobin/golangci-lint"
export STUB_GOBIN=$HOME/gobin
run_in "$WT"
want_rc 0
want '^a WARN multiple golangci-lint binaries can shadow'
want_not_f "$HOME"

begin_case "single golangci-lint binary in GOBIN"
fresh
export STUB_GOBIN=$STUBS
run_in "$WT"
want_rc 0
want '^a PASS '

# 21. outside a git repo
begin_case "outside git repo"
mkdir -p "$HOME/nogit"
run_in "$HOME/nogit"
want_rc 1
want '^c FAIL not inside a git repository'

# 2 (checked last, across all cases). only local toolchain probes
begin_case "go stub log"
[ -s "$STUB_LOG" ] || fail "go stub never called"
if grep -qvx 'local' "$STUB_LOG"; then
	RC=0 OUT=$(cat "$STUB_LOG")
	fail "go invoked with GOTOOLCHAIN other than local"
fi

# No module downloads or other network-capable go/git subcommands.
begin_case "no network subcommands"
if grep -nE '^(mod|get|install|run|generate) ' "$STUB_ARGV_LOG"; then
	RC=0 OUT=$(cat "$STUB_ARGV_LOG")
	fail "go invoked with a download/install subcommand"
fi
if grep -nE '^(fetch|pull|clone|push|ls-remote|remote|submodule|gc) ' "$STUB_GIT_LOG"; then
	RC=0 OUT=$(cat "$STUB_GIT_LOG")
	fail "git invoked with a network or mutating subcommand"
fi
[ -s "$STUB_GIT_LOG" ] || fail "git wrapper never called"

# 22. redaction: no fixture HOME in any output
begin_case "redaction"
if grep -qF "$HOME" "$ALL"; then
	RC=0 OUT=$(grep -nF "$HOME" "$ALL" | head -5)
	fail "output contains the literal HOME"
fi

# 23. no network or install commands in the scripts
begin_case "no install commands"
# (The pattern line itself is excluded.)
if grep -nE '(^|[;&|(`]|\$\()[[:space:]]*(go (install|get|mod download|run [^ ]*@)|curl |wget |asdf install|git (fetch|pull|clone|push)|docker (pull|compose up)|brew install)' "$CHECK" "$SELF" | grep -v 'grep -nE'; then
	RC=0 OUT=
	fail "found a forbidden command"
fi

# 24. syntax
begin_case "bash -n"
bash -n "$CHECK" || fail "bash -n env-check"
bash -n "$SELF" || fail "bash -n suite"
if command -v shellcheck >/dev/null 2>&1; then
	shellcheck "$CHECK" "$SELF" >/dev/null 2>&1 || echo "note: shellcheck reported findings (advisory)"
fi

echo "env-check-test: $N cases passed"
