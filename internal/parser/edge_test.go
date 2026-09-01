package parser

import (
	"errors"
	"strings"
	"testing"

	vcard "github.com/emersion/go-vcard"

	"github.com/fairbearlab/rolodex/internal/model"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated read failure")
}

// A reader error is reported, not returned as an empty address book.
func TestParseReaderError(t *testing.T) {
	_, _, err := Parse(failingReader{}, model.SourceUnknown)
	if err == nil || !strings.Contains(err.Error(), "reading vCard data") {
		t.Errorf("err = %v, want the read failure surfaced", err)
	}
}

// go-vcard's Card map can in principle hold a nil field; sanitizing must
// step over it rather than panic mid-parse.
func TestSanitizeCardNilField(t *testing.T) {
	card := vcard.Card{
		"FN": []*vcard.Field{nil, {Value: "A\x00B"}},
	}
	sanitizeCard(card)
	if got := card["FN"][1].Value; got != "AB" {
		t.Errorf("value = %q, want control byte stripped around the nil field", got)
	}
}

// TAB and the decoded "\n" escape are legitimate content (multi-line notes,
// tabular notes); stripping control characters must keep them.
func TestStripControlKeepsTabAndNewline(t *testing.T) {
	data := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:A\r\nNOTE:col1\tcol2\\nline2\r\nEND:VCARD\r\n"
	contacts, warnings, err := Parse(strings.NewReader(data), model.SourceUnknown)
	if err != nil || len(warnings) != 0 || len(contacts) != 1 {
		t.Fatalf("parse: %v, %d warnings, %d contacts", err, len(warnings), len(contacts))
	}
	if got := contacts[0].Note; got != "col1\tcol2\nline2" {
		t.Errorf("note = %q, want the tab and newline kept", got)
	}
}

// A PHOTO that is neither base64 nor a URI is opaque bytes and is kept as
// such rather than dropped.
func TestParsePhotoOpaqueValue(t *testing.T) {
	data := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:A\r\nPHOTO:opaque-value\r\nEND:VCARD\r\n"
	contacts, _, err := Parse(strings.NewReader(data), model.SourceUnknown)
	if err != nil || len(contacts) != 1 {
		t.Fatalf("parse: %v, %d contacts", err, len(contacts))
	}
	if got := string(contacts[0].Photo); got != "opaque-value" {
		t.Errorf("photo bytes = %q, want the raw value kept", got)
	}
	if contacts[0].PhotoURI != "" {
		t.Errorf("PhotoURI = %q, want empty for a non-URI value", contacts[0].PhotoURI)
	}
}
