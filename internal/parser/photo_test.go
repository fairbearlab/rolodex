package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

// A PHOTO given as a URI (Google's VALUE=uri, or a bare http/data value)
// is a reference, not image bytes.
func TestParsePhotoURI(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "in.vcf")
	data := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:A\r\nPHOTO;VALUE=uri:https://example.com/a.jpg\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:B\r\nPHOTO:https://example.com/b.png\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:C\r\nPHOTO;ENCODING=b;TYPE=JPEG:/9j/4A==\r\nEND:VCARD\r\n"
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	contacts, _, err := ParseFile(p, model.SourceUnknown)
	if err != nil || len(contacts) != 3 {
		t.Fatalf("parse: %v, %d contacts", err, len(contacts))
	}
	for i, want := range []string{"https://example.com/a.jpg", "https://example.com/b.png", ""} {
		if contacts[i].PhotoURI != want {
			t.Errorf("contact %d PhotoURI = %q, want %q", i, contacts[i].PhotoURI, want)
		}
		if (len(contacts[i].Photo) > 0) != (want == "") {
			t.Errorf("contact %d Photo bytes = %d, PhotoURI = %q: exactly one should be set", i, len(contacts[i].Photo), want)
		}
	}
}
