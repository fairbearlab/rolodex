package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeNamesakeVCF writes one contact whose name is the only thing it could
// share with another: a private phone, no email.
func writeNamesakeVCF(t *testing.T, path, given, family, tel string) {
	t.Helper()
	body := "BEGIN:VCARD\r\nVERSION:3.0\r\nN:" + family + ";" + given + ";;;\r\nFN:" + given + " " + family + "\r\nTEL:" + tel + "\r\nEND:VCARD\r\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestRunMergeRemovesItsOwnStaleReviewFile: the review path defaults to a
// file beside --out, and a stale-file removal that only fired for an explicit
// --review left the previous run's review.vcf in place when a re-run had
// nothing to review. report.json then said "review": [] while review.vcf
// still held 24 contacts, `rolodex review` printed "Nothing to review", and
// `rolodex resolve` refused the pair: "review.vcf has 24 contacts but report
// only references 0". A review.vcf that rolodex wrote (every card carries
// X-ROLODEX-REVIEW) is removed whether or not the user named the path; a
// bystander file at the derived path is still left alone
// (TestRunMergeKeepsUnrelatedReviewFile).
func TestRunMergeRemovesItsOwnStaleReviewFile(t *testing.T) {
	dir := t.TempDir()
	icloud := filepath.Join(dir, "icloud.vcf")
	google := filepath.Join(dir, "google.vcf")
	out := filepath.Join(dir, "merged.vcf")
	review := filepath.Join(dir, "review.vcf")

	// Run 1: two namesakes with nothing else in common are a near-name pair
	// and land in review.vcf.
	writeNamesakeVCF(t, icloud, "David", "Lee", "317-555-0001")
	writeNamesakeVCF(t, google, "David", "Lee", "415-555-0002")
	if err := runMerge([]string{"--icloud", icloud, "--google", google, "--out", out}); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	stale, err := os.ReadFile(filepath.Clean(review))
	if err != nil {
		t.Fatalf("run 1 wrote no review.vcf: %v", err)
	}
	if !strings.Contains(string(stale), "X-ROLODEX-REVIEW:true") {
		t.Fatalf("test precondition: run 1's review.vcf is not marked as rolodex output:\n%s", stale)
	}

	// Run 2: the Google export was fixed, nothing is left to review, and
	// --review is again left to its default.
	writeNamesakeVCF(t, google, "Bob", "Baker", "415-555-0002")
	if err := runMerge([]string{"--icloud", icloud, "--google", google, "--out", out}); err != nil {
		t.Fatalf("run 2: %v", err)
	}
	if _, err := os.Stat(review); err == nil {
		t.Errorf("stale %s from run 1 survived a run with nothing to review", review)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("merged output missing after run 2: %v", err)
	}
}
