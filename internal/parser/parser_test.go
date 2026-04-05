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
