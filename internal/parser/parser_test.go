package parser

import (
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

func TestParseBasicContact(t *testing.T) {
	input := `BEGIN:VCARD
VERSION:3.0
N:Smith;Robert;;;
FN:Robert Smith
EMAIL;TYPE=HOME:bob@gmail.com
TEL;TYPE=CELL:+1 (555) 123-4567
ORG:Acme Corp
TITLE:Engineer
BDAY:1985-03-15
NOTE:Old friend
URL:https://bobsmith.com
END:VCARD`

	contacts, warnings, err := Parse(strings.NewReader(input), model.SourceICloud)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}

	c := contacts[0]
	if c.Source != model.SourceICloud {
		t.Errorf("source = %q, want %q", c.Source, model.SourceICloud)
	}
	if c.FamilyName != "Smith" {
		t.Errorf("family name = %q, want %q", c.FamilyName, "Smith")
	}
	if c.GivenName != "Robert" {
		t.Errorf("given name = %q, want %q", c.GivenName, "Robert")
	}
	if c.FormattedName != "Robert Smith" {
		t.Errorf("formatted name = %q, want %q", c.FormattedName, "Robert Smith")
	}
	if len(c.Emails) != 1 || c.Emails[0].Address != "bob@gmail.com" {
		t.Errorf("emails = %v, want [bob@gmail.com]", c.Emails)
	}
	if c.Emails[0].Type != "HOME" {
		t.Errorf("email type = %q, want %q", c.Emails[0].Type, "HOME")
	}
	if len(c.Phones) != 1 || c.Phones[0].Number != "+1 (555) 123-4567" {
		t.Errorf("phones = %v", c.Phones)
	}
	if c.Org != "Acme Corp" {
		t.Errorf("org = %q, want %q", c.Org, "Acme Corp")
	}
	if c.Title != "Engineer" {
		t.Errorf("title = %q, want %q", c.Title, "Engineer")
	}
	if c.Birthday != "1985-03-15" {
		t.Errorf("birthday = %q, want %q", c.Birthday, "1985-03-15")
	}
	if c.Note != "Old friend" {
		t.Errorf("note = %q, want %q", c.Note, "Old friend")
	}
	if c.URL != "https://bobsmith.com" {
		t.Errorf("url = %q, want %q", c.URL, "https://bobsmith.com")
	}
}

func TestParseMultipleContacts(t *testing.T) {
	input := `BEGIN:VCARD
VERSION:3.0
N:One;Contact;;;
FN:Contact One
END:VCARD
BEGIN:VCARD
VERSION:3.0
N:Two;Contact;;;
FN:Contact Two
END:VCARD
BEGIN:VCARD
VERSION:3.0
N:Three;Contact;;;
FN:Contact Three
END:VCARD`

	contacts, _, err := Parse(strings.NewReader(input), model.SourceGoogle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 3 {
		t.Fatalf("expected 3 contacts, got %d", len(contacts))
	}
	if contacts[0].FamilyName != "One" {
		t.Errorf("first contact family = %q", contacts[0].FamilyName)
	}
	if contacts[2].FamilyName != "Three" {
		t.Errorf("third contact family = %q", contacts[2].FamilyName)
	}
}

func TestParseAddress(t *testing.T) {
	input := `BEGIN:VCARD
VERSION:3.0
N:Test;User;;;
FN:User Test
ADR;TYPE=HOME:;;123 Main St;Springfield;IL;62701;US
ADR;TYPE=WORK:;;456 Office Blvd;Chicago;IL;60601;US
END:VCARD`

	contacts, _, err := Parse(strings.NewReader(input), model.SourceICloud)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts[0].Addresses) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(contacts[0].Addresses))
	}
	home := contacts[0].Addresses[0]
	if home.Type != "HOME" || home.Street != "123 Main St" || home.City != "Springfield" {
		t.Errorf("home address = %+v", home)
	}
}

func TestParseEmptyInput(t *testing.T) {
	contacts, _, err := Parse(strings.NewReader(""), model.SourceICloud)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 0 {
		t.Errorf("expected 0 contacts, got %d", len(contacts))
	}
}

func TestParseMultipleEmails(t *testing.T) {
	input := `BEGIN:VCARD
VERSION:3.0
N:Multi;Email;;;
FN:Email Multi
EMAIL;TYPE=HOME:home@example.com
EMAIL;TYPE=WORK:work@example.com
EMAIL:other@example.com
END:VCARD`

	contacts, _, err := Parse(strings.NewReader(input), model.SourceGoogle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts[0].Emails) != 3 {
		t.Fatalf("expected 3 emails, got %d", len(contacts[0].Emails))
	}
}

func TestParseFile(t *testing.T) {
	contacts, _, err := ParseFile("../../testdata/icloud.vcf", model.SourceICloud)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 5 {
		t.Fatalf("expected 5 contacts, got %d", len(contacts))
	}
}

func TestParseRestoresProvenanceSource(t *testing.T) {
	vcf := "BEGIN:VCARD\nVERSION:3.0\nFN:A\nN:A;;;;\nX-ROLODEX-SOURCE:icloud\nEND:VCARD\n" +
		"BEGIN:VCARD\nVERSION:3.0\nFN:B\nN:B;;;;\nX-ROLODEX-SOURCE:google\nEND:VCARD\n" +
		"BEGIN:VCARD\nVERSION:3.0\nFN:C\nN:C;;;;\nX-ROLODEX-SOURCE:merged(icloud+google)\nEND:VCARD\n" +
		"BEGIN:VCARD\nVERSION:3.0\nFN:D\nN:D;;;;\nEND:VCARD\n"
	contacts, _, err := Parse(strings.NewReader(vcf), "review")
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Source{model.SourceICloud, model.SourceGoogle, "review", "review"}
	for i, w := range want {
		if contacts[i].Source != w {
			t.Errorf("contact %d source = %q, want %q", i, contacts[i].Source, w)
		}
	}
	// Merged provenance stays in Extra for resolve to read.
	if got := contacts[2].Extra["X-ROLODEX-SOURCE"]; len(got) != 1 || got[0] != "merged(icloud+google)" {
		t.Errorf("X-ROLODEX-SOURCE not preserved in Extra: %v", got)
	}
	if got := contacts[0].Extra["X-ROLODEX-SOURCE"]; len(got) != 1 || got[0] != "icloud" {
		t.Errorf("single-source X-ROLODEX-SOURCE not preserved in Extra: %v", got)
	}
}

func TestParseNormalizesOrgAndBirthday(t *testing.T) {
	vcf := "BEGIN:VCARD\nVERSION:3.0\nFN:A\nN:A;;;;\nORG:Kunkels Drive-In;\nBDAY:1989-06-29\nEND:VCARD\n" +
		"BEGIN:VCARD\nVERSION:3.0\nFN:B\nN:B;;;;\nORG:Kunkels Drive-In\nBDAY:19890629\nEND:VCARD\n" +
		"BEGIN:VCARD\nVERSION:3.0\nFN:C\nN:C;;;;\nORG:;FRIEND\nBDAY;X-APPLE-OMIT-YEAR=1604:1604-10-26\nEND:VCARD\n" +
		"BEGIN:VCARD\nVERSION:3.0\nFN:D\nN:D;;;;\nBDAY:--1026\nEND:VCARD\n"
	contacts, _, err := Parse(strings.NewReader(vcf), model.SourceICloud)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 4 {
		t.Fatalf("expected 4 contacts, got %d", len(contacts))
	}
	if contacts[0].Org != "Kunkels Drive-In" || contacts[1].Org != "Kunkels Drive-In" {
		t.Errorf("ORG not normalized: %q vs %q", contacts[0].Org, contacts[1].Org)
	}
	if contacts[2].Org != ";FRIEND" {
		t.Errorf("ORG with empty leading component = %q, want ;FRIEND (position preserved)", contacts[2].Org)
	}
	if contacts[0].Birthday != "1989-06-29" || contacts[1].Birthday != "1989-06-29" {
		t.Errorf("BDAY not normalized: %q vs %q", contacts[0].Birthday, contacts[1].Birthday)
	}
	if contacts[2].Birthday != "--10-26" {
		t.Errorf("iCloud omit-year BDAY = %q, want --10-26", contacts[2].Birthday)
	}
	if contacts[3].Birthday != "--10-26" {
		t.Errorf("Google no-year BDAY = %q, want --10-26", contacts[3].Birthday)
	}
}

func TestParseIgnoresProvenanceOnIngest(t *testing.T) {
	// A provider export that carries a stale X-ROLODEX-SOURCE (a rolodex
	// output re-imported and re-exported) must keep the flag's label.
	vcf := "BEGIN:VCARD\nVERSION:3.0\nFN:A\nN:A;;;;\nX-ROLODEX-SOURCE:google\nEND:VCARD\n"
	contacts, _, err := Parse(strings.NewReader(vcf), model.SourceICloud)
	if err != nil {
		t.Fatal(err)
	}
	if contacts[0].Source != model.SourceICloud {
		t.Errorf("ingest source = %q, want icloud (flag is authoritative)", contacts[0].Source)
	}
	contacts, _, _ = Parse(strings.NewReader(vcf), model.SourceUnknown)
	if contacts[0].Source != model.SourceGoogle {
		t.Errorf("read-back source = %q, want google (restored from X-ROLODEX-SOURCE)", contacts[0].Source)
	}
}

// TestParseAppleOmitYearNonPlaceholderYear covers the X-APPLE-OMIT-YEAR
// parameter path for a placeholder year other than Apple's usual 1604 (which
// normalize.Birthday already strips on its own).
func TestParseAppleOmitYearNonPlaceholderYear(t *testing.T) {
	vcf := "BEGIN:VCARD\nVERSION:3.0\nFN:A\nN:A;;;;\nBDAY;X-APPLE-OMIT-YEAR=1900:1900-10-26\nEND:VCARD\n" +
		"BEGIN:VCARD\nVERSION:3.0\nFN:B\nN:B;;;;\nBDAY;X-APPLE-OMIT-YEAR=1900:1989-10-26\nEND:VCARD\n" +
		"BEGIN:VCARD\nVERSION:3.0\nFN:C\nN:C;;;;\nBDAY:1989-10-26\nEND:VCARD\n"
	contacts, _, err := Parse(strings.NewReader(vcf), model.SourceICloud)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 3 {
		t.Fatalf("expected 3 contacts, got %d", len(contacts))
	}
	// The omit-year parameter matches the value's year: drop the year.
	if contacts[0].Birthday != "--10-26" {
		t.Errorf("omit-year birthday = %q, want --10-26", contacts[0].Birthday)
	}
	// The parameter names a different year than the value carries: the value
	// is a real birthday and must keep its year.
	if contacts[1].Birthday != "1989-10-26" {
		t.Errorf("mismatched omit-year birthday = %q, want 1989-10-26", contacts[1].Birthday)
	}
	// No parameter at all: unchanged.
	if contacts[2].Birthday != "1989-10-26" {
		t.Errorf("plain birthday = %q, want 1989-10-26", contacts[2].Birthday)
	}
}

// A .vcf is untrusted input. Control characters in a field value are never
// contact data and are dangerous twice: the review TUI writes values straight
// to the terminal and truncates with an ANSI-aware helper that preserves
// escape sequences, so a hostile name could clear and repaint the screen and
// hide the card the reviewer is about to merge; and a bare CR survives the
// writer (which escapes LF but not CR), forging a property line for any reader
// that treats a lone CR as a line break.
func TestParseStripsControlCharacters(t *testing.T) {
	in := "BEGIN:VCARD\r\nVERSION:3.0\r\n" +
		"FN:Bob\x1b[2J\x1b]0;pwned\x07\x1b[31mEVIL\r\n" +
		"N:Doe;Bob\x1b[31m;;;\r\n" +
		"NOTE:hi\rX-ROLODEX-REVIEW:true\r\n" +
		"ORG:Acme\x1b[0m\r\n" +
		"TEL;TYPE=WORK\x1b[0m:+1 555 0100\r\n" +
		"EMAIL:a\x07@b.com\r\n" +
		"END:VCARD\r\n"

	contacts, _, err := Parse(strings.NewReader(in), model.SourceICloud)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}
	c := contacts[0]

	values := map[string]string{
		"FormattedName": c.FormattedName,
		"GivenName":     c.GivenName,
		"Note":          c.Note,
		"Org":           c.Org,
		"phone type":    c.Phones[0].Type,
		"email":         c.Emails[0].Address,
	}
	for field, v := range values {
		for _, bad := range []struct {
			name string
			r    rune
		}{{"ESC", 0x1b}, {"BEL", 0x07}, {"CR", '\r'}} {
			if strings.ContainsRune(v, bad.r) {
				t.Errorf("%s = %q still carries %s; it would reach the terminal and the .vcf writer",
					field, v, bad.name)
			}
		}
	}

	// The surrounding text is kept — sanitizing must not discard the contact.
	if !strings.Contains(c.FormattedName, "Bob") || !strings.Contains(c.FormattedName, "EVIL") {
		t.Errorf("FormattedName = %q, want the printable text preserved", c.FormattedName)
	}

	// A legitimate multi-line NOTE survives: go-vcard decodes the "\n" escape
	// into a real newline, and that is real contact data.
	multi := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:X\r\nN:X;;;;\r\nNOTE:line1\\nline2\r\nEND:VCARD\r\n"
	cs, _, err := Parse(strings.NewReader(multi), model.SourceICloud)
	if err != nil {
		t.Fatalf("Parse multiline: %v", err)
	}
	if !strings.Contains(cs[0].Note, "\n") {
		t.Errorf("Note = %q, want the newline in a multi-line note preserved", cs[0].Note)
	}
}

// Invisible and direction-changing characters must not reach the review card.
// lipgloss scores them as zero width, so the TUI reserves no columns for them:
// a right-to-left override in one card's name reorders the OTHER card's name
// and email on the same rendered row, and the reviewer merges two people whose
// cards were made to look alike. ZWJ and ZWNJ are kept — they are load-bearing
// in Indic and Persian names and in emoji.
func TestParseStripsBidiAndZeroWidth(t *testing.T) {
	ch := func(r rune) string { return string(r) }
	in := "BEGIN:VCARD\r\nVERSION:3.0\r\n" +
		"FN:" + ch(0x202e) + "evil" + ch(0x2066) + "spoof" + ch(0xfeff) + "\r\n" +
		"N:Doe;" + ch(0x202e) + "John" + ch(0x200b) + ";;;\r\n" +
		"NOTE:ok" + ch(0x200d) + "joiner " + ch(0x200c) + "nonjoiner\r\n" +
		"END:VCARD\r\n"

	contacts, _, err := Parse(strings.NewReader(in), model.SourceICloud)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	c := contacts[0]

	for _, bad := range []struct {
		name string
		r    rune
	}{
		{"RLO", 0x202e}, {"isolate", 0x2066}, {"BOM", 0xfeff}, {"ZWSP", 0x200b},
	} {
		if strings.ContainsRune(c.FormattedName+c.GivenName, bad.r) {
			t.Errorf("%s (U+%04X) reached the review card", bad.name, bad.r)
		}
	}
	if c.FormattedName != "evilspoof" || c.GivenName != "John" {
		t.Errorf("FormattedName=%q GivenName=%q, want the visible text preserved", c.FormattedName, c.GivenName)
	}
	if !strings.ContainsRune(c.Note, 0x200d) || !strings.ContainsRune(c.Note, 0x200c) {
		t.Errorf("Note = %q, want ZWJ and ZWNJ preserved", c.Note)
	}
}
