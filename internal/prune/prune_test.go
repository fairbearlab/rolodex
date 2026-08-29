package prune

import (
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

func TestReachable(t *testing.T) {
	cases := []struct {
		name string
		c    model.ParsedContact
		by   []Channel
		want bool
	}{
		{"email", model.ParsedContact{Emails: []model.Email{{Address: "a@b.com"}}}, DefaultChannels, true},
		{"phone", model.ParsedContact{Phones: []model.Phone{{Number: "+1 (415) 555-0123"}}}, DefaultChannels, true},
		{"address street", model.ParsedContact{Addresses: []model.Address{{Street: "1 Main St"}}}, DefaultChannels, true},
		{"address postcode only", model.ParsedContact{Addresses: []model.Address{{PostCode: "80202"}}}, DefaultChannels, true},
		{"address country only is a picker default", model.ParsedContact{Addresses: []model.Address{{Country: "US"}}}, DefaultChannels, false},
		{"address city and region only", model.ParsedContact{Addresses: []model.Address{{City: "Denver", Region: "CO"}}}, DefaultChannels, false},
		{"address all whitespace", model.ParsedContact{Addresses: []model.Address{{Street: "  ", City: "\t", Type: "HOME"}}}, DefaultChannels, false},
		{"placeholder phone", model.ParsedContact{Phones: []model.Phone{{Number: "000-000-0000"}}}, DefaultChannels, false},
		{"placeholder email", model.ParsedContact{Emails: []model.Email{{Address: "unknown"}}}, DefaultChannels, false},
		{"second email is real", model.ParsedContact{Emails: []model.Email{{Address: "unknown"}, {Address: "real@example.org"}}}, DefaultChannels, true},
		{"names only", model.ParsedContact{FormattedName: "Just A Name", GivenName: "Just", FamilyName: "Name"}, DefaultChannels, false},
		{"org and photo are not channels", model.ParsedContact{Org: "Acme", Photo: []byte{1}, Birthday: "1990-01-01"}, DefaultChannels, false},
		{"url off by default", model.ParsedContact{URL: "https://facebook.com/x"}, DefaultChannels, false},
		{"url opt-in", model.ParsedContact{URL: "https://facebook.com/x"}, []Channel{ChannelEmail, ChannelPhone, ChannelAddress, ChannelURL}, true},
		{"blank url opt-in", model.ParsedContact{URL: "   "}, []Channel{ChannelURL}, false},
		{"address disabled", model.ParsedContact{Addresses: []model.Address{{Street: "1 Main St"}}}, []Channel{ChannelEmail, ChannelPhone}, false},
		{"email disabled", model.ParsedContact{Emails: []model.Email{{Address: "a@b.com"}}}, []Channel{ChannelPhone}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Reachable(tc.c, tc.by); got != tc.want {
				t.Errorf("Reachable(%+v, %v) = %v, want %v", tc.c, tc.by, got, tc.want)
			}
		})
	}
}

func TestParseChannels(t *testing.T) {
	t.Run("valid list", func(t *testing.T) {
		got, err := ParseChannels("email,phone")
		if err != nil {
			t.Fatal(err)
		}
		want := []Channel{ChannelEmail, ChannelPhone}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("all four with spaces and case", func(t *testing.T) {
		got, err := ParseChannels(" Email, phone ,ADDRESS,url ")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 4 || got[3] != ChannelURL {
			t.Errorf("got %v", got)
		}
	})
	t.Run("duplicates tolerated", func(t *testing.T) {
		got, err := ParseChannels("email,email,phone")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("got %v, want the two distinct channels", got)
		}
	})
	t.Run("unknown names the valid four", func(t *testing.T) {
		_, err := ParseChannels("email,fax")
		if err == nil {
			t.Fatal("expected an error for fax")
		}
		for _, want := range []string{`"fax"`, "email", "phone", "address", "url"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q lacks %q", err, want)
			}
		}
	})
	t.Run("empty", func(t *testing.T) {
		if _, err := ParseChannels(""); err == nil {
			t.Error("expected an error for an empty list")
		}
		if _, err := ParseChannels(" , "); err == nil {
			t.Error("expected an error for a list of blanks")
		}
	})
}

func TestSplitTracksIndexAndOrder(t *testing.T) {
	contacts := []model.ParsedContact{
		{FormattedName: "OK", Emails: []model.Email{{Address: "a@b.com"}}},
		{FormattedName: "Bad"},
		{FormattedName: "OK2", Phones: []model.Phone{{Number: "415-555-0102"}}},
		{FormattedName: "Bad2", Org: "Corp"},
	}
	r := Split(contacts, Options{ReachableBy: DefaultChannels})
	if r.Total != 4 {
		t.Errorf("Total = %d, want 4", r.Total)
	}
	if len(r.Kept) != 2 || r.Kept[0].FormattedName != "OK" || r.Kept[1].FormattedName != "OK2" {
		t.Errorf("Kept = %v", names(r.Kept))
	}
	if len(r.Removed) != 2 || r.Removed[0].FormattedName != "Bad" || r.Removed[1].FormattedName != "Bad2" {
		t.Errorf("Removed = %v", names(r.Removed))
	}
	if len(r.Detail) != 2 {
		t.Fatalf("Detail has %d entries, want one per removed contact", len(r.Detail))
	}
	if r.Detail[0].Index != 1 || r.Detail[0].Name != "Bad" {
		t.Errorf("Detail[0] = %+v, want index 1 / Bad", r.Detail[0])
	}
	if r.Detail[1].Index != 3 || r.Detail[1].Name != "Bad2" || !r.Detail[1].HasOrg {
		t.Errorf("Detail[1] = %+v, want index 3 / Bad2 / has_org", r.Detail[1])
	}
}

func TestSplitEmpty(t *testing.T) {
	r := Split(nil, Options{ReachableBy: DefaultChannels})
	if r.Total != 0 || len(r.Kept) != 0 || len(r.Removed) != 0 || len(r.Detail) != 0 {
		t.Errorf("Split(nil) = %+v, want all empty", r)
	}
}

// Every Removed flag describes presence, independent of which channels are
// enabled: a contact appears with has_address:true only when address is not
// an enabled channel (with it enabled the contact would be kept).
func TestSplitRemovedFlags(t *testing.T) {
	c := model.ParsedContact{
		FormattedName: "Everything But Reach",
		Org:           "Acme",
		Title:         "CTO",
		Addresses:     []model.Address{{City: "Denver"}}, // present, not deliverable
		URL:           "https://facebook.com/x",
		Birthday:      "1980-05-06",
		Photo:         []byte{0xff, 0xd8},
	}
	r := Split([]model.ParsedContact{c}, Options{ReachableBy: []Channel{ChannelEmail, ChannelPhone}})
	if len(r.Detail) != 1 {
		t.Fatalf("want 1 removed, got %d", len(r.Detail))
	}
	d := r.Detail[0]
	want := Removed{Name: "Everything But Reach", Index: 0, HasOrg: true, HasTitle: true,
		HasAddress: true, HasURL: true, HasBirthday: true, HasPhoto: true}
	if d != want {
		t.Errorf("Detail[0] = %+v, want %+v", d, want)
	}

	bare := Split([]model.ParsedContact{{FormattedName: "Bare"}}, Options{ReachableBy: DefaultChannels}).Detail[0]
	if bare != (Removed{Name: "Bare"}) {
		t.Errorf("bare contact flags = %+v, want all false", bare)
	}

	// A city-only address is not deliverable, but it is not nothing either.
	city := Split([]model.ParsedContact{{FormattedName: "City", Addresses: []model.Address{{City: "Paris", Country: "France"}}}},
		Options{ReachableBy: DefaultChannels})
	if len(city.Removed) != 1 || !city.Detail[0].HasAddress {
		t.Errorf("city-only address: removed=%d has_address=%v, want removed with has_address", len(city.Removed), city.Detail[0].HasAddress)
	}

	// A whitespace-only address and a blank URL are absent, not present.
	blank := Split([]model.ParsedContact{{FormattedName: "Blank", Addresses: []model.Address{{Street: " "}}, URL: " "}},
		Options{ReachableBy: DefaultChannels}).Detail[0]
	if blank.HasAddress || blank.HasURL {
		t.Errorf("blank address/URL reported as present: %+v", blank)
	}
}

func TestSplitAddressEnabledKeeps(t *testing.T) {
	c := model.ParsedContact{FormattedName: "Address Only", Addresses: []model.Address{{Street: "1 Main"}}}
	r := Split([]model.ParsedContact{c}, Options{ReachableBy: DefaultChannels})
	if len(r.Kept) != 1 || len(r.Removed) != 0 {
		t.Errorf("address-only contact with address enabled: kept=%d removed=%d", len(r.Kept), len(r.Removed))
	}
}

func TestSplitContactNameFallback(t *testing.T) {
	tests := []struct {
		contact model.ParsedContact
		want    string
	}{
		{model.ParsedContact{FormattedName: "Alice"}, "Alice"},
		{model.ParsedContact{GivenName: "Bob", FamilyName: "Smith"}, "Bob Smith"},
		{model.ParsedContact{FamilyName: "Jones"}, "Jones"},
		{model.ParsedContact{Org: "Acme"}, "Acme"},
		{model.ParsedContact{}, "(unknown)"},
	}
	for _, tt := range tests {
		r := Split([]model.ParsedContact{tt.contact}, Options{ReachableBy: DefaultChannels})
		if len(r.Detail) != 1 {
			t.Fatalf("expected 1 removed for %+v", tt.contact)
		}
		if r.Detail[0].Name != tt.want {
			t.Errorf("name = %q, want %q", r.Detail[0].Name, tt.want)
		}
	}
}

func names(cs []model.ParsedContact) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.FormattedName
	}
	return out
}

// An email or phone that did not count, and a note, are reported: "name
// only" must mean exactly that.
func TestSplitReportsUncountedEmailPhoneAndNote(t *testing.T) {
	c := model.ParsedContact{
		FormattedName: "Short Code",
		Phones:        []model.Phone{{Number: "611"}},
		Emails:        []model.Email{{Address: "unknown"}},
		Note:          "cable company support line",
	}
	r := Split([]model.ParsedContact{c}, Options{ReachableBy: DefaultChannels})
	if len(r.Detail) != 1 {
		t.Fatalf("want 1 removed, got %d", len(r.Detail))
	}
	want := Removed{Name: "Short Code", HasEmail: true, HasPhone: true, HasNote: true}
	if r.Detail[0] != want {
		t.Errorf("Detail[0] = %+v, want %+v", r.Detail[0], want)
	}
	uri := Split([]model.ParsedContact{{FormattedName: "Pic", PhotoURI: "https://example.com/p.jpg"}}, Options{ReachableBy: DefaultChannels}).Detail[0]
	if !uri.HasPhoto {
		t.Error("a photo given as a URI is a photo")
	}
}
