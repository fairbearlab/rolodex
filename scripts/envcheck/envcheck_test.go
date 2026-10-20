// Package envcheck is a test-only package: it runs the shell fixture suite for
// bin/env-check as part of go test, so the suite executes in CI without any
// Makefile or workflow change.
package envcheck

import (
	"os"
	"os/exec"
	"testing"
)

const (
	envCheck = "../../bin/env-check"
	suite    = "../env-check-test.sh"
)

func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			t.Fatalf("%s not found on PATH in CI: %v", name, err)
		}
		t.Skipf("%s not found on PATH: %v", name, err)
	}
}

func TestEnvCheckSuite(t *testing.T) {
	requireTool(t, "bash")
	requireTool(t, "git")

	// Reading both files makes go test's result cache depend on them, so a
	// cached PASS cannot hide a changed script.
	if b, err := os.ReadFile(envCheck); err != nil || len(b) == 0 {
		t.Fatalf("cannot read bin/env-check: %v", err)
	}
	if b, err := os.ReadFile(suite); err != nil || len(b) == 0 {
		t.Fatalf("cannot read scripts/env-check-test.sh: %v", err)
	}

	if out, err := exec.Command("bash", "-n", envCheck).CombinedOutput(); err != nil {
		t.Fatalf("bash -n bin/env-check: %v\n%s", err, out)
	}
	if out, err := exec.Command("bash", "-n", suite).CombinedOutput(); err != nil {
		t.Fatalf("bash -n scripts/env-check-test.sh: %v\n%s", err, out)
	}
	if out, err := exec.Command("bash", suite).CombinedOutput(); err != nil {
		t.Fatalf("env-check suite failed: %v\n%s", err, out)
	}
}
