package writer

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	vcard "github.com/emersion/go-vcard"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/parser"
)

// failingWriter errors on every Write call, simulating a full disk or a
// closed pipe partway through a multi-contact write.
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("simulated write failure")
}

// TestWriteReturnsErrorOnWriteFailure covers the write error path: Write
// formats each contact into a buffer and emits it in one call, and a failure
// writing that buffer out must be reported, not silently swallowed.
func TestWriteReturnsErrorOnWriteFailure(t *testing.T) {
	mc := model.MergedContact{
		Contact: model.ParsedContact{GivenName: "A", FamilyName: "B", FormattedName: "A B"},
	}
	err := Write(failingWriter{}, []model.MergedContact{mc})
	if err == nil {
		t.Fatal("expected an error when the underlying writer fails, got nil")
	}
	if !strings.Contains(err.Error(), "writing contact") {
		t.Errorf("error = %q, want it to name the failing contact", err.Error())
	}
}

// TestWriteStopsAtFirstFailure: a writer that fails only after N bytes
// (mid-second-contact) must still surface the error rather than continuing
// to "succeed" on later contacts.
func TestWriteStopsAtFirstFailure(t *testing.T) {
	mc := func(name string) model.MergedContact {
		return model.MergedContact{Contact: model.ParsedContact{GivenName: name, FormattedName: name}}
	}
	err := Write(failingWriter{}, []model.MergedContact{mc("First"), mc("Second")})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestWriteBasicContact(t *testing.T) {
	mc := model.MergedContact{
		Contact: model.ParsedContact{
			GivenName:     "Robert",
			FamilyName:    "Smith",
			FormattedName: "Robert Smith",
			Emails:        []model.Email{{Address: "bob@gmail.com", Type: "HOME"}},
			Phones:        []model.Phone{{Number: "+1 (555) 123-4567", Type: "CELL"}},
			Org:           "Acme Corp",
		},
		Sources: []model.Source{model.SourceICloud, model.SourceGoogle},
		Score:   0.95,
	}

	var buf bytes.Buffer
	if err := Write(&buf, []model.MergedContact{mc}); err != nil {
		t.Fatalf("write error: %v", err)
	}

	output := buf.String()

	// Should be valid vCard 3.0
	if !strings.Contains(output, "VERSION:3.0") {
		t.Error("missing VERSION:3.0")
	}
	if !strings.Contains(output, "FN:Robert Smith") {
		t.Error("missing FN")
	}
	if !strings.Contains(output, "X-ROLODEX-SOURCE:merged(icloud+google)") {
		t.Errorf("missing provenance field, got:\n%s", output)
	}
	if !strings.Contains(output, "X-ROLODEX-SCORE:0.95") {
		t.Errorf("missing score field, got:\n%s", output)
	}
}

func TestWriteRoundTrip(t *testing.T) {
	mc := model.MergedContact{
		Contact: model.ParsedContact{
			GivenName:     "Alice",
			FamilyName:    "Johnson",
			FormattedName: "Alice Johnson",
			Emails: []model.Email{
				{Address: "alice@example.com", Type: "HOME"},
				{Address: "alice@work.com", Type: "WORK"},
			},
			Phones: []model.Phone{
				{Number: "555-1234", Type: "CELL"},
			},
			Birthday: "1990-05-15",
			Note:     "Test contact",
		},
		Sources: []model.Source{model.SourceICloud},
	}

	var buf bytes.Buffer
	if err := Write(&buf, []model.MergedContact{mc}); err != nil {
		t.Fatalf("write error: %v", err)
	}

	// Re-parse the output to verify it's valid vCard
	dec := vcard.NewDecoder(&buf)
	card, err := dec.Decode()
	if err != nil {
		t.Fatalf("re-parse error: %v", err)
	}

	if card.PreferredValue(vcard.FieldFormattedName) != "Alice Johnson" {
		t.Errorf("FN after re-parse = %q", card.PreferredValue(vcard.FieldFormattedName))
	}
}

func TestWriteReviewFlag(t *testing.T) {
	mc := model.MergedContact{
		Contact: model.ParsedContact{
			GivenName:     "Test",
			FamilyName:    "User",
			FormattedName: "Test User",
		},
		Sources:    []model.Source{model.SourceGoogle},
		ReviewFlag: true,
	}

	var buf bytes.Buffer
	if err := Write(&buf, []model.MergedContact{mc}); err != nil {
		t.Fatalf("write error: %v", err)
	}

	if !strings.Contains(buf.String(), "X-ROLODEX-REVIEW:true") {
		t.Error("missing X-ROLODEX-REVIEW field")
	}
}

// TestWriteKeepsEscapedSemicolon: go-vcard decodes "ORG:Acme\; Inc." to the
// value `Acme\; Inc.` and would re-encode it as `Acme\\; Inc.`, which every
// reader takes as org "Acme\" plus unit "Inc.". The writer must emit the
// escape exactly once, and the value must survive a parse/write cycle.
func TestWriteKeepsEscapedSemicolon(t *testing.T) {
	in := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:A\r\nN:A;;;;\r\nORG:Acme\\; Inc.\r\nNOTE:x\\;y\r\nEND:VCARD\r\n"
	contacts, _, err := parser.Parse(strings.NewReader(in), model.SourceICloud)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := contacts[0].Org; got != `Acme\; Inc.` {
		t.Fatalf("parsed ORG = %q, want the escape kept as one component", got)
	}

	var buf bytes.Buffer
	if err := Write(&buf, []model.MergedContact{{Contact: contacts[0], Sources: []model.Source{model.SourceICloud}}}); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ORG:Acme\\; Inc.\r\n") || strings.Contains(out, `\\;`) {
		t.Errorf("ORG escape not preserved on write:\n%s", out)
	}
	if !strings.Contains(out, "NOTE:x\\;y\r\n") {
		t.Errorf("NOTE escape not preserved on write:\n%s", out)
	}

	again, _, err := parser.Parse(strings.NewReader(out), model.SourceICloud)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if again[0].Org != contacts[0].Org || again[0].Note != contacts[0].Note {
		t.Errorf("round trip changed ORG %q -> %q, NOTE %q -> %q", contacts[0].Org, again[0].Org, contacts[0].Note, again[0].Note)
	}
}
