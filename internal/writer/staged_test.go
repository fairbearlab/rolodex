package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

// WriteBytes carries the JSON reports, so it must land the exact bytes and
// replace whatever was at the destination.
func TestWriteBytesReplacesDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteBytes(path, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("WriteBytes: %v", err)
	}
	got, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"ok":true}` {
		t.Errorf("content = %q", got)
	}
	// No staging temp file left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory not clean after commit: %v", entries)
	}
}

// A bare filename has no directory component; staging must use "." rather
// than fail or stage somewhere surprising.
func TestStageBytesBareFilename(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := WriteBytes("out.vcf", []byte("data")); err != nil {
		t.Fatalf("WriteBytes with a bare filename: %v", err)
	}
	got, err := os.ReadFile("out.vcf")
	if err != nil || string(got) != "data" {
		t.Errorf("read back %q, %v", got, err)
	}
}

// A rename that fails must report the error and leave the destination as it
// was, not half-replaced.
func TestCommitFailureLeavesDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "final.vcf")
	if err := os.WriteFile(path, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := StageBytes(path, []byte("next"))
	if err != nil {
		t.Fatalf("StageBytes: %v", err)
	}
	// Pull the staged file out from under Commit to force the rename to fail.
	if err := os.Remove(s.tmp); err != nil {
		t.Fatal(err)
	}
	if err := s.Commit(); err == nil {
		t.Fatal("Commit succeeded with no staged file")
	}
	got, err := os.ReadFile(filepath.Clean(path))
	if err != nil || string(got) != "previous" {
		t.Errorf("destination after failed commit = %q, %v; want the previous content untouched", got, err)
	}
}

// TYPE parameters ride along on emails and phones that have one and are
// absent, not empty, on those that do not.
func TestWriteTypeParams(t *testing.T) {
	mc := model.MergedContact{Contact: model.ParsedContact{
		FormattedName: "A B",
		Emails:        []model.Email{{Address: "home@example.com", Type: "HOME"}, {Address: "bare@example.com"}},
		Phones:        []model.Phone{{Number: "555-0100", Type: "CELL"}, {Number: "555-0101"}},
	}}
	var sb strings.Builder
	if err := Write(&sb, []model.MergedContact{mc}); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{
		"EMAIL;TYPE=HOME:home@example.com\r\n",
		"EMAIL:bare@example.com\r\n",
		"TEL;TYPE=CELL:555-0100\r\n",
		"TEL:555-0101\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "TYPE=:") || strings.Contains(out, ";TYPE=\r\n") {
		t.Errorf("an empty TYPE parameter was emitted:\n%s", out)
	}
}

// A photo held as a URI goes out verbatim under VALUE=uri — not escaped, not
// base64-encoded — with the TYPE only when there is one. Image bytes win
// over a URI when both are somehow present.
func TestWritePhotoURI(t *testing.T) {
	uri := "https://example.com/a,b.jpg"
	withType := model.MergedContact{Contact: model.ParsedContact{
		FormattedName: "A", PhotoURI: uri, PhotoType: "JPEG",
	}}
	bare := model.MergedContact{Contact: model.ParsedContact{
		FormattedName: "B", PhotoURI: uri,
	}}
	bytesWin := model.MergedContact{Contact: model.ParsedContact{
		FormattedName: "C", Photo: []byte{0xFF, 0xD8}, PhotoURI: uri, PhotoType: "JPEG",
	}}
	var sb strings.Builder
	if err := Write(&sb, []model.MergedContact{withType, bare, bytesWin}); err != nil {
		t.Fatal(err)
	}
	cards := strings.SplitAfter(sb.String(), "END:VCARD\r\n")
	if len(cards) < 3 {
		t.Fatalf("expected 3 cards, got:\n%s", sb.String())
	}
	if want := "PHOTO;VALUE=uri;TYPE=JPEG:" + uri + "\r\n"; !strings.Contains(cards[0], want) {
		t.Errorf("card A lacks %q:\n%s", want, cards[0])
	}
	if want := "PHOTO;VALUE=uri:" + uri + "\r\n"; !strings.Contains(cards[1], want) {
		t.Errorf("card B lacks %q:\n%s", want, cards[1])
	}
	if !strings.Contains(cards[2], "PHOTO;ENCODING=b;TYPE=JPEG:") || strings.Contains(cards[2], "VALUE=uri") {
		t.Errorf("card C should carry the image bytes, not the URI:\n%s", cards[2])
	}
}

// WriteBytes surfaces a staging failure — here a destination directory that
// does not exist — instead of committing nothing silently.
func TestWriteBytesStagingFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "report.json")
	if err := WriteBytes(path, []byte("data")); err == nil {
		t.Fatal("WriteBytes into a missing directory succeeded")
	}
}
