package writer

import (
	"bytes"
	"strings"
	"testing"

	vcard "github.com/emersion/go-vcard"

	"github.com/fairbearlabs/rolodex/internal/model"
)

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
