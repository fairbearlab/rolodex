package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStderr runs fn with os.Stderr redirected to a pipe and returns what
// was written. reportParseWarnings writes to os.Stderr directly.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	func() {
		defer func() { os.Stderr = orig; _ = w.Close() }()
		fn()
	}()
	return <-done
}

// TestRunMergeReportsMalformedEntries: a truncated export (an interrupted
// download, a full disk) used to shrink the address book silently. The
// decoder skipped the broken card, the "N contacts loaded" line was counted
// after the loss, and the command exited 0. The pipeline must name the
// skipped entries on stderr, as audit already did.
func TestRunMergeReportsMalformedEntries(t *testing.T) {
	dir := t.TempDir()
	icloud := filepath.Join(dir, "icloud.vcf")
	google := filepath.Join(dir, "google.vcf")
	out := filepath.Join(dir, "merged.vcf")

	writeNamesakeVCF(t, icloud, "David", "Lee", "317-555-0001")
	// A complete card followed by one cut off mid-property: the decoder
	// reports the second as malformed ("no END field found").
	good := "BEGIN:VCARD\r\nVERSION:3.0\r\nN:Baker;Bob;;;\r\nFN:Bob Baker\r\nTEL:415-555-0002\r\nEND:VCARD\r\n"
	truncated := "BEGIN:VCARD\r\nVERSION:3.0\r\nN:Chen;Kath"
	if err := os.WriteFile(google, []byte(good+truncated), 0o600); err != nil {
		t.Fatal(err)
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = runMerge([]string{"--icloud", icloud, "--google", google, "--out", out})
	})
	if runErr != nil {
		t.Fatalf("merge: %v", runErr)
	}

	for _, want := range []string{
		"warning: 1 malformed entries in " + google + " were skipped and are NOT in the output",
		"entry 1: malformed vCard entry:",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, icloud) {
		t.Errorf("stderr warns about the intact iCloud export:\n%s", stderr)
	}

	// The intact contacts still ship; the truncated one is absent, as warned.
	merged, err := os.ReadFile(filepath.Clean(out))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(merged), "FN:Bob Baker") || strings.Contains(string(merged), "Chen") {
		t.Errorf("merged output does not match the warning:\n%s", merged)
	}
}
