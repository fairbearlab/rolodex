package main

import (
	"os"
	"path/filepath"
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
