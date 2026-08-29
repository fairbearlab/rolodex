package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

// A card with no END:VCARD made go-vcard read on into the next card, so
// the two came back as one contact with the second's fields grafted on and
// no warning: total was one short, warning_count was 0, and prune --out
// wrote the mangled card with exit 0. It is now skipped with a warning.
func TestParseWarnsOnMissingEND(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "in.vcf")
	data := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Alice One\r\nEMAIL:alice@example.com\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Bob Two\r\nEMAIL:bob@example.com\r\n" + // no END
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Carol Three\r\nEMAIL:carol@example.com\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Dan Four\r\nEMAIL:dan@example.com\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Eve Five\r\nEMAIL:eve@" // truncated at EOF
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	contacts, warnings, err := ParseFile(p, model.SourceUnknown)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 2 || contacts[0].FormattedName != "Alice One" || contacts[1].FormattedName != "Dan Four" {
		t.Errorf("contacts = %d, want Alice and Dan only", len(contacts))
	}
	for _, c := range contacts {
		if len(c.Emails) != 1 {
			t.Errorf("%s has %d emails; a stranger's was grafted on", c.FormattedName, len(c.Emails))
		}
	}
	if len(warnings) != 2 || warnings[0].Index != 1 || !strings.Contains(warnings[0].Message, "missing END:VCARD") ||
		!strings.Contains(warnings[0].Message, "1 card(s)") {
		t.Errorf("warnings = %+v, want the first at index 1 naming the missing END and the absorbed card", warnings)
	}
	// The absorbed card was a physical entry too: Eve is the fifth card in
	// the file (index 4), not the fourth.
	if len(warnings) == 2 && warnings[1].Index != 4 {
		t.Errorf("truncated card reported at index %d, want 4", warnings[1].Index)
	}
}
