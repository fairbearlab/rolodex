package writer

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/parser"
)

// appleCard carries every custom label the way Apple exports it: a group
// prefix ties the X-ABLabel to the property it names.
const appleCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nN:Lee;Ann;;;\r\nFN:Ann Lee\r\n" +
	"item1.EMAIL;TYPE=INTERNET:ann@school.example\r\nitem1.X-ABLabel:School\r\n" +
	"item2.TEL:+1 212 555 0100\r\nitem2.X-ABLabel:Studio\r\n" +
	"item3.ADR:;;1 Main St;Springfield;;;\r\nitem3.X-ABADR:us\r\n" +
	"item4.URL:https://ann.example\r\nitem4.X-ABLabel:_$!<HomePage>!$_\r\n" +
	"item5.X-ABRELATEDNAMES:Jo Lee\r\nitem5.X-ABLabel:_$!<Sister>!$_\r\n" +
	"EMAIL:ann@home.example\r\nEND:VCARD\r\n"

func roundTrip(t *testing.T, vcf string) string {
	t.Helper()
	contacts, warnings, err := parser.Parse(strings.NewReader(vcf), model.SourceUnknown)
	if err != nil || len(warnings) > 0 || len(contacts) != 1 {
		t.Fatalf("parse: %v %v %d", err, warnings, len(contacts))
	}
	var buf bytes.Buffer
	if err := Write(&buf, []model.MergedContact{{Contact: contacts[0]}}); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// go-vcard strips the group before the parser sees a field, and the model had
// nowhere to keep it, so every written card came out with bare EMAIL and a
// detached X-ABLABEL that labelled nothing.
func TestWritePreservesPropertyGroups(t *testing.T) {
	out := roundTrip(t, appleCard)
	for _, want := range []string{
		"item1.EMAIL;TYPE=INTERNET:ann@school.example\r\n",
		"item1.X-ABLABEL:School\r\n",
		"item2.TEL:+1 212 555 0100\r\n",
		"item2.X-ABLABEL:Studio\r\n",
		"item3.ADR:;;1 Main St;Springfield;;;\r\n",
		"item3.X-ABADR:us\r\n",
		"item4.URL:https://ann.example\r\n",
		"item4.X-ABLABEL:_$!<HomePage>!$_\r\n",
		"item5.X-ABRELATEDNAMES:Jo Lee\r\n",
		"item5.X-ABLABEL:_$!<Sister>!$_\r\n",
		"\r\nEMAIL:ann@home.example\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\r\nX-ABLABEL") {
		t.Errorf("output has a label detached from its group:\n%s", out)
	}
}

// A label whose property is gone (deduplicated away by a merge) labels
// nothing, so it is not written.
func TestWriteDropsOrphanedLabels(t *testing.T) {
	c := model.ParsedContact{
		FormattedName: "Ann Lee",
		Emails:        []model.Email{{Address: "ann@school.example", Group: "item1"}},
		Extra: map[string][]string{
			"item1.X-ABLABEL": {"School"},
			"item7.X-ABLABEL": {"Old"},
			"item7.X-ABADR":   {"us"},
		},
	}
	var buf bytes.Buffer
	if err := Write(&buf, []model.MergedContact{{Contact: c}}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "item1.X-ABLABEL:School") {
		t.Errorf("label with its property was dropped:\n%s", out)
	}
	if strings.Contains(out, "item7") {
		t.Errorf("orphaned label was written:\n%s", out)
	}
}
