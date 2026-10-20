package model

import "testing"

func TestRegroupRenamesCollidingGroups(t *testing.T) {
	base := ParsedContact{
		Emails: []Email{{Address: "a@x", Group: "item1"}},
		Extra:  map[string][]string{"item1.X-ABLABEL": {"School"}},
	}
	other := ParsedContact{
		Emails:    []Email{{Address: "b@x", Group: "ITEM1"}},
		Phones:    []Phone{{Number: "1", Group: "item2"}},
		Addresses: []Address{{Street: "s", Group: "item1"}},
		URL:       "u", URLGroup: "item1",
		Extra: map[string][]string{"item1.X-ABLABEL": {"Work"}, "item2.X-ABLABEL": {"Cell"}, "NICKNAME": {"B"}},
	}
	used := make(map[string]bool)
	_ = Regroup(base, used)
	got := Regroup(other, used)

	// item1 collides (case-insensitively) and becomes the first free name;
	// item2 does not collide and is kept.
	if got.Emails[0].Group != "item3" || got.Addresses[0].Group != "item3" || got.URLGroup != "item3" {
		t.Errorf("groups = %q %q %q, want item3", got.Emails[0].Group, got.Addresses[0].Group, got.URLGroup)
	}
	if got.Phones[0].Group != "item2" {
		t.Errorf("phone group = %q, want item2 kept", got.Phones[0].Group)
	}
	if v := got.Extra["item3.X-ABLABEL"]; len(v) != 1 || v[0] != "Work" {
		t.Errorf("extra = %v, want Work relabelled under item3", got.Extra)
	}
	if got.Extra["NICKNAME"] == nil || got.Extra["item2.X-ABLABEL"] == nil || got.Extra["item1.X-ABLABEL"] != nil {
		t.Errorf("extra = %v", got.Extra)
	}
	// The caller's contact is not modified.
	if other.Emails[0].Group != "ITEM1" || other.Extra["item1.X-ABLABEL"] == nil {
		t.Errorf("Regroup modified its input: %+v", other)
	}
	// A contact with no collision comes back unchanged.
	if c := Regroup(ParsedContact{Emails: []Email{{Group: "item9"}}}, used); c.Emails[0].Group != "item9" {
		t.Errorf("non-colliding group renamed to %q", c.Emails[0].Group)
	}
}

func TestSplitGroup(t *testing.T) {
	for key, want := range map[string][2]string{
		"item1.X-ABLABEL": {"item1", "X-ABLABEL"},
		"NICKNAME":        {"", "NICKNAME"},
	} {
		if g, n := SplitGroup(key); g != want[0] || n != want[1] {
			t.Errorf("SplitGroup(%q) = %q, %q, want %v", key, g, n, want)
		}
	}
}
