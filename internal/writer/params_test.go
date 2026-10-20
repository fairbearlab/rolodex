package writer

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/parser"
)

// A quoted TYPE value may hold ':' and ';', which the parameter escaper does
// not touch. Written back unquoted, the ':' ended the parameters early and
// readers took "EVIL@ATTACKER.TEST:real@good.example" as the address.
func TestTypeParamCannotBreakOutOfParameters(t *testing.T) {
	vcf := "BEGIN:VCARD\r\nVERSION:3.0\r\nN:Lee;Ann;;;\r\nFN:Ann Lee\r\n" +
		"EMAIL;TYPE=\"X:evil@attacker.test,\":real@good.example\r\n" +
		"TEL;TYPE=\"CELL;X=1\":+1 212 555 0100\r\n" +
		"ADR;TYPE=\"HOME:x\":;;1 Main St;Springfield;;;\r\n" +
		"EMAIL;TYPE=work:ann@work.example\r\n" +
		"END:VCARD\r\n"
	contacts, _, err := parser.Parse(strings.NewReader(vcf), model.SourceUnknown)
	if err != nil || len(contacts) != 1 {
		t.Fatalf("parse: %v, %d contacts", err, len(contacts))
	}
	var buf bytes.Buffer
	if err := Write(&buf, []model.MergedContact{{Contact: contacts[0]}}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"\r\nEMAIL:real@good.example\r\n",
		"\r\nTEL:+1 212 555 0100\r\n",
		"\r\nADR:;;1 Main St;Springfield;;;\r\n",
		"\r\nEMAIL;TYPE=WORK:ann@work.example\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "evil") {
		t.Errorf("injected TYPE value reached the output:\n%s", out)
	}
}

// The writer does not trust the model either: a TYPE or PhotoType that can
// end the parameter list is not written.
func TestWriterDropsUnsafeParams(t *testing.T) {
	c := model.ParsedContact{
		FormattedName: "Ann Lee",
		Emails:        []model.Email{{Address: "ann@good.example", Type: "X:evil"}},
		Phones:        []model.Phone{{Number: "2125550100", Type: "CELL\"x"}},
		Photo:         []byte("img"), PhotoType: "JPEG;X=1",
	}
	var buf bytes.Buffer
	if err := Write(&buf, []model.MergedContact{{Contact: c}}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"\r\nEMAIL:ann@good.example\r\n", "\r\nTEL:2125550100\r\n", "\r\nPHOTO;ENCODING=b:aW1n\r\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	// The photo-URI branch carries its own copy of the check.
	uri := model.ParsedContact{FormattedName: "Ann Lee", PhotoURI: "https://example.test/a.jpg", PhotoType: "JPEG:x"}
	buf.Reset()
	if err := Write(&buf, []model.MergedContact{{Contact: uri}}); err != nil {
		t.Fatal(err)
	}
	if want := "\r\nPHOTO;VALUE=uri:https://example.test/a.jpg\r\n"; !strings.Contains(buf.String(), want) {
		t.Errorf("output lacks %q:\n%s", want, buf.String())
	}
	for i, r := range []rune{'\r', '\n', ';', ':', '"', ','} {
		if isSafeParam("A" + string(r) + "B") {
			t.Errorf("case %d: %q accepted as a safe parameter value", i, r)
		}
	}
	if !isSafeParam("image/jpeg") || !isSafeParam("") {
		t.Error("plain values rejected")
	}
}
