package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunMergeDefaultsReviewPathNextToOut covers the --review default added
// alongside --out: when --review is omitted, review.vcf lands beside the
// merged output rather than in the process working directory.
func TestRunMergeDefaultsReviewPathNextToOut(t *testing.T) {
	tests := []struct {
		name string
		// outDir is created under t.TempDir(); outName is the --out basename.
		outSubdir string
	}{
		{"out in a nested directory", "nested"},
		{"out in the top directory", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			icloud := filepath.Join(dir, "icloud.vcf")
			google := filepath.Join(dir, "google.vcf")
			writeTestVCF(t, icloud, "Alpha", "alpha@example.com")
			writeTestVCF(t, google, "Alpha", "alpha.other@example.com")

			outDir := dir
			if tt.outSubdir != "" {
				outDir = filepath.Join(dir, tt.outSubdir)
				if err := os.MkdirAll(outDir, 0o750); err != nil {
					t.Fatalf("mkdir %s: %v", outDir, err)
				}
			}
			out := filepath.Join(outDir, "merged.vcf")

			// --review deliberately omitted.
			if err := runMerge([]string{"--icloud", icloud, "--google", google, "--out", out}); err != nil {
				t.Fatalf("runMerge: %v", err)
			}

			wantReview := filepath.Join(outDir, "review.vcf")
			if _, err := os.Stat(wantReview); err != nil {
				t.Errorf("expected review.vcf beside --out at %s: %v", wantReview, err)
			}
			if _, err := os.Stat(out); err != nil {
				t.Errorf("expected merged output at %s: %v", out, err)
			}
			// It must NOT have been written to the working directory.
			if _, err := os.Stat("review.vcf"); err == nil {
				t.Error("review.vcf was written to the working directory, not beside --out")
				_ = os.Remove("review.vcf")
			}
		})
	}
}

// TestRunMergeExplicitReviewPathWins confirms the default does not override
// an explicit --review.
func TestRunMergeExplicitReviewPathWins(t *testing.T) {
	dir := t.TempDir()
	icloud := filepath.Join(dir, "icloud.vcf")
	google := filepath.Join(dir, "google.vcf")
	writeTestVCF(t, icloud, "Alpha", "alpha@example.com")
	writeTestVCF(t, google, "Alpha", "alpha.other@example.com")

	out := filepath.Join(dir, "out", "merged.vcf")
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	explicit := filepath.Join(dir, "elsewhere.vcf")

	if err := runMerge([]string{
		"--icloud", icloud, "--google", google, "--out", out, "--review", explicit,
	}); err != nil {
		t.Fatalf("runMerge: %v", err)
	}

	if _, err := os.Stat(explicit); err != nil {
		t.Errorf("expected review output at the explicit path %s: %v", explicit, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(out), "review.vcf")); err == nil {
		t.Error("default review.vcf was written even though --review was explicit")
	}
}

// TestRunMergeRequiresBothSources covers the guard that precedes the default.
func TestRunMergeRequiresBothSources(t *testing.T) {
	dir := t.TempDir()
	icloud := filepath.Join(dir, "icloud.vcf")
	writeTestVCF(t, icloud, "Alpha", "alpha@example.com")

	if err := runMerge([]string{"--icloud", icloud}); err == nil {
		t.Error("expected an error when --google is missing, got nil")
	}
	if err := runMerge([]string{"--google", icloud}); err == nil {
		t.Error("expected an error when --icloud is missing, got nil")
	}
}

// TestRunMergeRejectsCollidingOutputs pins the guard on the derived --review
// default. "--out dir/review.vcf" aimed both writes at one file: pipeline.go
// wrote the merged contacts and then overwrote them with the review set, so
// the merged output vanished with no error. run.go already refused its
// reserved paths this way; merge did not.
func TestRunMergeRejectsCollidingOutputs(t *testing.T) {
	dir := t.TempDir()
	icloud := filepath.Join(dir, "icloud.vcf")
	google := filepath.Join(dir, "google.vcf")
	writeTestVCF(t, icloud, "Alpha", "alpha@example.com")
	writeTestVCF(t, google, "Alpha", "alpha.other@example.com")

	cases := []struct {
		name string
		args []string
	}{
		{
			"derived review path collides with --out",
			[]string{"--out", filepath.Join(dir, "review.vcf")},
		},
		{
			"explicit --review equals --out",
			[]string{"--out", filepath.Join(dir, "m.vcf"), "--review", filepath.Join(dir, "m.vcf")},
		},
		{
			"--report equals --out",
			[]string{"--out", filepath.Join(dir, "m.vcf"), "--report", filepath.Join(dir, "m.vcf")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--icloud", icloud, "--google", google}, tc.args...)
			err := runMerge(args)
			if err == nil {
				t.Fatal("runMerge accepted colliding output paths; one write would silently destroy the other")
			}
			if !strings.Contains(err.Error(), "must be different") {
				t.Errorf("error = %v, want it to name the collision", err)
			}
		})
	}
}

// TestRunMergeKeepsUnrelatedReviewFile pins the deletion guard. When a run
// produces no review-tier pairs, merge clears a stale review.vcf — but the
// path is now derived from --out, so "merge --out ~/Documents/merged.vcf"
// deleted ~/Documents/review.vcf, a file this run never wrote and the user
// never named.
func TestRunMergeKeepsUnrelatedReviewFile(t *testing.T) {
	dir := t.TempDir()
	icloud := filepath.Join(dir, "icloud.vcf")
	google := filepath.Join(dir, "google.vcf")
	// Identical contacts with a shared email auto-merge, so nothing is queued
	// for review and merge reaches the removal branch.
	writeTestVCF(t, icloud, "Alpha", "alpha@example.com")
	writeTestVCF(t, google, "Alpha", "alpha@example.com")

	bystander := filepath.Join(dir, "review.vcf")
	const content = "a file the user wrote, not rolodex\n"
	if err := os.WriteFile(bystander, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "merged.vcf")
	if err := runMerge([]string{"--icloud", icloud, "--google", google, "--out", out}); err != nil {
		t.Fatalf("runMerge: %v", err)
	}

	got, err := os.ReadFile(filepath.Clean(bystander))
	if err != nil {
		t.Fatalf("rolodex deleted %s, a path it derived and the user never named: %v", bystander, err)
	}
	if string(got) != content {
		t.Errorf("bystander file = %q, want it untouched (%q)", got, content)
	}
}

// TestRunMergeRejectsAliasedAndInputPaths pins the rest of the path guard.
// filepath.Abs cleans a path but does not case-fold, and macOS APFS/HFS+ are
// case-insensitive by default: "--out Merged.vcf --review merged.vcf" named
// one file that looked like two, and the stale-review removal then deleted the
// merged output while the command exited 0. Writing over an input export
// destroyed the one artifact worth keeping if the merge went wrong, and
// writer.WriteFile stages through "<path>.tmp", so that sibling is reserved.
func TestRunMergeRejectsAliasedAndInputPaths(t *testing.T) {
	dir := t.TempDir()
	icloud := filepath.Join(dir, "icloud.vcf")
	google := filepath.Join(dir, "google.vcf")
	writeTestVCF(t, icloud, "Alpha", "alpha@example.com")
	writeTestVCF(t, google, "Alpha", "alpha@example.com")

	cases := []struct {
		name string
		args []string
	}{
		{"case-folded alias of --out", []string{
			"--out", filepath.Join(dir, "Merged.vcf"), "--review", filepath.Join(dir, "merged.vcf")}},
		{"--out over the iCloud input", []string{
			"--out", icloud, "--review", filepath.Join(dir, "r.vcf")}},
		{"--review over the Google input", []string{
			"--out", filepath.Join(dir, "m.vcf"), "--review", google}},
		{"--out collides with --review staging file", []string{
			"--out", filepath.Join(dir, "a.vcf.tmp"), "--review", filepath.Join(dir, "a.vcf")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--icloud", icloud, "--google", google}, tc.args...)
			if err := runMerge(args); err == nil {
				t.Fatal("runMerge accepted aliased paths; one write would silently destroy another file")
			}
		})
	}

	// The inputs must still be intact after every rejected run.
	for _, p := range []string{icloud, google} {
		data, err := os.ReadFile(filepath.Clean(p))
		if err != nil {
			t.Fatalf("input %s was destroyed: %v", p, err)
		}
		if !strings.Contains(string(data), "Alpha") {
			t.Errorf("input %s was overwritten: %q", p, data)
		}
	}
}
