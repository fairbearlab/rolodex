package writer

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/parser"
)

// TestWriteEscapesValuesForReaders: the model holds decoded values, so the
// writer owns the escaping. A literal ';' inside an N or ADR component must
// go out as `\;`, and a literal backslash as `\\` — a whole-buffer rewrite
// of `\\;` into `\;` turned family `Smith\` + given "John" into the single
// family "Smith;John" for Apple and Google. ORG is already wire form and is
// emitted as is; so are unmodeled passthrough fields.
func TestWriteEscapesValuesForReaders(t *testing.T) {
	contacts := []model.MergedContact{
		{Contact: model.ParsedContact{
			FamilyName: "O;Brien", GivenName: "Sean", FormattedName: "Sean O;Brien",
			Addresses: []model.Address{{Type: "HOME", Street: "1 Main; Apt 2", City: "Town"}},
			Note:      "x;y, z\nnext",
			Title:     "VP, Sales; EMEA",
			URL:       `https://x.test/a;b\c`,
			Org:       `Acme\; Inc.;R\\D`,
			Extra:     map[string][]string{"X-CUSTOM": {`a\;b\,c\\d`}},
		}, Sources: []model.Source{model.SourceICloud}},
		{Contact: model.ParsedContact{
			FamilyName: `Smith\`, GivenName: "John", FormattedName: "John Smith",
			Note: `C:\;temp`,
		}, Sources: []model.Source{model.SourceGoogle}},
	}

	var buf bytes.Buffer
	if err := Write(&buf, contacts); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`N:O\;Brien;Sean;;;`,
		`FN:Sean O\;Brien`,
		`ADR;TYPE=HOME:;;1 Main\; Apt 2;Town;;;`,
		`NOTE:x\;y\, z\nnext`,
		`TITLE:VP\, Sales\; EMEA`,
		`URL:https://x.test/a\;b\\c`,
		`ORG:Acme\; Inc.;R\\D`,
		`X-CUSTOM:a\;b\,c\\d`,
		`N:Smith\\;John;;;`,
		`NOTE:C:\\\;temp`,
	} {
		if !strings.Contains(out, want+"\r\n") {
			t.Errorf("output lacks line %q:\n%s", want, out)
		}
	}

	// And the real parser reads back exactly what was written.
	again, warns, err := parser.Parse(strings.NewReader(out), model.SourceUnknown)
	if err != nil || len(warns) != 0 || len(again) != 2 {
		t.Fatalf("re-parse: err=%v warnings=%v contacts=%d", err, warns, len(again))
	}
	for i, c := range contacts {
		got, want := again[i], c.Contact
		if got.FamilyName != want.FamilyName || got.GivenName != want.GivenName || got.FormattedName != want.FormattedName ||
			got.Note != want.Note || got.Org != want.Org || got.Title != want.Title || got.URL != want.URL {
			t.Errorf("contact %d changed in the round trip:\n got %+v\nwant %+v", i, got, want)
		}
		if len(want.Addresses) > 0 && (len(got.Addresses) != 1 || got.Addresses[0] != want.Addresses[0]) {
			t.Errorf("contact %d address changed: got %+v want %+v", i, got.Addresses, want.Addresses)
		}
		for k, v := range want.Extra {
			if g := got.Extra[k]; len(g) != 1 || g[0] != v[0] {
				t.Errorf("contact %d %s = %q, want %q", i, k, g, v)
			}
		}
	}
}
