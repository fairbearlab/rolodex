package parser

import (
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

// crlf joins vCard lines with the CRLF terminator. Raw strings keep the
// backslashes in the wire form literal, which is the point of these tests.
func crlf(lines ...string) string {
	return strings.Join(lines, "\r\n") + "\r\n"
}

// TestParseDecodesEscapesInStructuredFields: N and ADR were split on every
// ';', escaped or not, so `N:O\;Brien;Sean;;;` became family `O\`, given
// "Brien", middle "Sean". A literal backslash was worse: go-vcard's decoder
// turns `\\` into `\` but leaves `\;` alone, so `N:Smith\\;John;;;` (family
// `Smith\`, given "John") and `N:Smith\;John;;;` (one family "Smith;John")
// were the same string by the time the parser looked at them. The parser
// must see the wire form and decode each component itself.
func TestParseDecodesEscapesInStructuredFields(t *testing.T) {
	input := crlf(
		`BEGIN:VCARD`,
		`VERSION:3.0`,
		`N:O\;Brien;Sean;;;`,
		`FN:Sean O\; Brien`,
		`ADR;TYPE=HOME:;;1 Main\; Apt 2;Town;;;`,
		`NOTE:x\;y`,
		`END:VCARD`,
		`BEGIN:VCARD`,
		`VERSION:3.0`,
		`N:Smith\\;John;;;`,
		`FN:John Smith`,
		`NOTE:C:\\;temp`,
		`END:VCARD`,
	)

	contacts, warns, err := Parse(strings.NewReader(input), model.SourceICloud)
	if err != nil || len(warns) != 0 || len(contacts) != 2 {
		t.Fatalf("Parse: err=%v warnings=%v contacts=%d", err, warns, len(contacts))
	}

	obrien := contacts[0]
	if obrien.FamilyName != "O;Brien" || obrien.GivenName != "Sean" || obrien.MiddleName != "" {
		t.Errorf("N with escaped separator: family=%q given=%q middle=%q, want O;Brien / Sean / empty",
			obrien.FamilyName, obrien.GivenName, obrien.MiddleName)
	}
	if obrien.FormattedName != "Sean O; Brien" {
		t.Errorf("FN = %q, want the escape decoded", obrien.FormattedName)
	}
	if len(obrien.Addresses) != 1 || obrien.Addresses[0].Street != "1 Main; Apt 2" || obrien.Addresses[0].City != "Town" {
		t.Errorf("ADR with escaped separator = %+v, want street `1 Main; Apt 2`, city Town", obrien.Addresses)
	}
	if obrien.Note != "x;y" {
		t.Errorf("NOTE = %q, want x;y", obrien.Note)
	}

	smith := contacts[1]
	if smith.FamilyName != `Smith\` || smith.GivenName != "John" {
		t.Errorf("N with a literal backslash before the separator: family=%q given=%q, want `Smith\\` / John",
			smith.FamilyName, smith.GivenName)
	}
	if smith.Note != `C:\;temp` {
		t.Errorf("NOTE with a literal backslash = %q, want %q", smith.Note, `C:\;temp`)
	}
}

// TestParseKeepsOrgInWireForm pins the one structured field that is not
// decoded: ORG is held as its wire form (escapes intact) because it is a
// single string in the model and "Acme; Inc." would be indistinguishable
// from the two units "Acme" and " Inc.". normalize.DisplayComponents reads
// it. Unmodeled fields pass through as wire form too.
func TestParseKeepsOrgInWireForm(t *testing.T) {
	input := crlf(
		`BEGIN:VCARD`,
		`VERSION:3.0`,
		`N:A;;;;`,
		`FN:A`,
		`ORG:Acme\; Inc.;R\\D`,
		`X-CUSTOM:a\;b\,c\\d`,
		`END:VCARD`,
	)
	contacts, _, err := Parse(strings.NewReader(input), model.SourceICloud)
	if err != nil {
		t.Fatal(err)
	}
	if got := contacts[0].Org; got != `Acme\; Inc.;R\\D` {
		t.Errorf("ORG = %q, want the wire form kept", got)
	}
	if got := contacts[0].Extra["X-CUSTOM"]; len(got) != 1 || got[0] != `a\;b\,c\\d` {
		t.Errorf("X-CUSTOM = %q, want the wire form kept for passthrough", got)
	}
}

// TestParseUnfoldsBeforeDecodingEscapes: a fold may split an escape
// sequence. The backslash at the end of one physical line and the "n" at the
// start of the next are one newline escape.
func TestParseUnfoldsBeforeDecodingEscapes(t *testing.T) {
	input := crlf(
		`BEGIN:VCARD`,
		`VERSION:3.0`,
		`N:A;;;;`,
		`FN:A`,
		`NOTE:line one\`,
		` nline two`,
		`END:VCARD`,
	)
	contacts, _, err := Parse(strings.NewReader(input), model.SourceICloud)
	if err != nil {
		t.Fatal(err)
	}
	if got := contacts[0].Note; got != "line one\nline two" {
		t.Errorf("NOTE = %q, want the escape that straddled the fold decoded as a newline", got)
	}
}
