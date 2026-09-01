package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

func bob() []model.MergedContact {
	return []model.MergedContact{{Contact: model.ParsedContact{FormattedName: "Bob"}}}
}

// The old staging path "<path>.tmp" was opened in place, so a symlink
// planted there was written through and then renamed onto the output.
// Staging now creates a random dot-file exclusively; a planted path is
// never opened.
func TestStageNeverOpensPlantedTempPath(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(victim, []byte("KEEP ME\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "kept.vcf")
	if err := os.Symlink(victim, out+".tmp"); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(out, bob()); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Clean(victim)); string(b) != "KEEP ME\n" {
		t.Fatalf("wrote through the planted symlink; victim.txt is now %q", b)
	}
	fi, err := os.Lstat(out)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		t.Errorf("output is not a regular file: %v %v", fi, err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("output mode = %o, want 0600", fi.Mode().Perm())
	}
	if b, _ := os.ReadFile(filepath.Clean(out)); !strings.Contains(string(b), "FN:Bob") {
		t.Errorf("output lacks the contact:\n%s", b)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Errorf("unexpected files in %s: %v", dir, entries)
	}
}

// Stage touches nothing at the destination; Abort discards the staged
// file; Commit replaces the destination and leaves no staging file.
func TestStageAbortAndCommit(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.vcf")
	if err := os.WriteFile(out, []byte("OLD\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Stage(out, bob())
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Clean(out)); string(b) != "OLD\n" {
		t.Error("Stage touched the destination")
	}
	s.Abort()
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("Abort left files behind: %v", entries)
	}
	if b, _ := os.ReadFile(filepath.Clean(out)); string(b) != "OLD\n" {
		t.Error("Abort touched the destination")
	}

	s, err = Stage(out, bob())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Commit(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Clean(out)); !strings.Contains(string(b), "FN:Bob") {
		t.Errorf("Commit did not replace the destination:\n%s", b)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("Commit left files behind: %v", entries)
	}
	s.Abort() // safe after Commit
	if _, err := os.Stat(out); err != nil {
		t.Errorf("Abort after Commit removed the output: %v", err)
	}

	if _, err := Stage(filepath.Join(dir, "no-such-dir", "out.vcf"), bob()); err == nil {
		t.Error("Stage into a missing directory succeeded")
	}
}

// A destination that is a directory is refused; os.Rename would otherwise
// replace an empty one with the file ("--out ~/Contacts").
func TestStageRefusesDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Contacts")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(target, bob()); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("error = %v, want a refusal", err)
	}
	if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
		t.Errorf("the directory was replaced: %v %v", fi, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("staging file left behind: %v", entries)
	}
}

// "merged" and "review" are the labels resolve and review parse with; they
// say where the parser was called from, not where a contact came from.
func TestWriteNoStampForReadBackLabels(t *testing.T) {
	for _, label := range []model.Source{"merged", "review"} {
		var buf strings.Builder
		if err := Write(&buf, []model.MergedContact{{Contact: model.ParsedContact{FormattedName: "X"}, Sources: []model.Source{label}}}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(buf.String(), "X-ROLODEX-SOURCE") {
			t.Errorf("%q was stamped as provenance:\n%s", label, buf.String())
		}
	}
}
