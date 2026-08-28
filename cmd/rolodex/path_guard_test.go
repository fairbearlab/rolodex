package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunRejectsOutputOverInput: `run` had only a check that its outputs did
// not collide with each other. `rolodex run --icloud icloud.vcf --google
// google.vcf --out icloud.vcf` overwrote the iCloud export with the resolved
// output and exited 0 — the pipeline reads both inputs before it writes, so
// nothing failed. merge had already grown this guard; run and resolve had not.
func TestRunRejectsOutputOverInput(t *testing.T) {
	dir := t.TempDir()
	icloud := filepath.Join(dir, "icloud.vcf")
	google := filepath.Join(dir, "google.vcf")
	writeTestVCF(t, icloud, "Alpha", "alpha@example.com")
	writeTestVCF(t, google, "Beta", "beta@example.com")
	before, err := os.ReadFile(filepath.Clean(icloud))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		out  string
		rep  string
		keep bool
	}{
		{"--out over --icloud", icloud, "", false},
		{"--report over --google", filepath.Join(dir, "final.vcf"), google, false},
		{"--keep would copy review.vcf over --google", filepath.Join(dir, "sub", "final.vcf"), "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.keep {
				// The Google export lives where --keep will copy review.vcf.
				sub := filepath.Join(dir, "sub")
				if err := os.MkdirAll(sub, 0o750); err != nil {
					t.Fatal(err)
				}
				google = filepath.Join(sub, "review.vcf")
				writeTestVCF(t, google, "Beta", "beta@example.com")
			}
			err := run(icloud, google, tc.out, tc.rep, tc.keep)
			if err == nil {
				t.Fatal("run succeeded; expected a path collision error before anything was written")
			}
			if !strings.Contains(err.Error(), "same file") {
				t.Errorf("error = %q, want a same-file collision", err)
			}
			after, err := os.ReadFile(filepath.Clean(icloud))
			if err != nil {
				t.Fatalf("icloud export is gone: %v", err)
			}
			if string(after) != string(before) {
				t.Errorf("icloud export was rewritten:\n%s", after)
			}
		})
	}
}

// TestResolveRejectsOutputOverInput: resolve reads --merged and --review and
// then writes --out through the same atomic rename, so --out naming either of
// them replaced an input with the output.
func TestResolveRejectsOutputOverInput(t *testing.T) {
	dir := t.TempDir()
	report := filepath.Join(dir, "report.json")
	review := filepath.Join(dir, "review.vcf")
	merged := filepath.Join(dir, "merged.vcf")
	for _, p := range []string{report, review, merged} {
		if err := os.WriteFile(p, []byte("placeholder\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, out := range []string{merged, review, report, strings.ToUpper(merged[:len(merged)-4]) + ".vcf"} {
		err := runResolve([]string{"--report", report, "--review", review, "--merged", merged, "--out", out})
		if err == nil {
			t.Fatalf("--out %s: resolve succeeded; expected a collision error", out)
		}
		if !strings.Contains(err.Error(), "same file") {
			t.Errorf("--out %s: error = %q, want a same-file collision", out, err)
		}
	}
}
