package merger

import (
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
)

// Both exports number their groups from item1. Unioned as is, the Google
// card's item1.X-ABLabel became a second label on the iCloud card's item1
// email; its own email lost its label.
func TestMergeClusterRenumbersCollidingGroups(t *testing.T) {
	contacts := []model.NormalizedContact{
		normalize.Contact(model.ParsedContact{Source: model.SourceICloud, GivenName: "Ann", FamilyName: "Lee",
			Emails: []model.Email{{Address: "ann@school.example", Group: "item1"}},
			Extra:  map[string][]string{"item1.X-ABLABEL": {"School"}}}),
		normalize.Contact(model.ParsedContact{Source: model.SourceGoogle, GivenName: "Ann", FamilyName: "Lee",
			Emails: []model.Email{{Address: "ann@work.example", Group: "item1"}},
			URL:    "https://ann.example", URLGroup: "item2",
			Extra: map[string][]string{"item1.X-ABLABEL": {"Studio"}, "item2.X-ABLABEL": {"Site"}}}),
	}
	mc := mergeCluster(contacts, []int{0, 1}, 0.9).Contact

	groupOf := map[string]string{}
	for _, e := range mc.Emails {
		groupOf[e.Address] = e.Group
	}
	school, work := groupOf["ann@school.example"], groupOf["ann@work.example"]
	if school == "" || work == "" || school == work {
		t.Fatalf("email groups = %v, want two distinct groups", groupOf)
	}
	if v := mc.Extra[school+".X-ABLABEL"]; len(v) != 1 || v[0] != "School" {
		t.Errorf("label of %s = %v, want [School]", school, v)
	}
	if v := mc.Extra[work+".X-ABLABEL"]; len(v) != 1 || v[0] != "Studio" {
		t.Errorf("label of %s = %v, want [Studio]", work, v)
	}
	if v := mc.Extra[mc.URLGroup+".X-ABLABEL"]; mc.URLGroup == "" || len(v) != 1 || v[0] != "Site" {
		t.Errorf("URL group %q label = %v, want [Site]", mc.URLGroup, v)
	}
	// The input contacts are not rewritten.
	if contacts[1].Parsed.Emails[0].Group != "item1" {
		t.Errorf("input contact regrouped in place: %+v", contacts[1].Parsed.Emails)
	}
}

// With no iCloud member the first contact is the base; it must not be
// unioned with itself, or its grouped properties come out twice.
func TestMergeClusterWithoutICloudDoesNotDuplicateGroups(t *testing.T) {
	g := func(email string) model.NormalizedContact {
		return normalize.Contact(model.ParsedContact{Source: model.SourceGoogle, GivenName: "Ann", FamilyName: "Lee",
			Emails: []model.Email{{Address: email}},
			Extra:  map[string][]string{"item1.X-ABRELATEDNAMES": {"Jo"}}})
	}
	contacts := []model.NormalizedContact{g("a@x.example"), g("b@x.example")}
	mc := mergeCluster(contacts, []int{0, 1}, 0.9).Contact
	var related int
	for k, v := range mc.Extra {
		if _, name := model.SplitGroup(k); name == "X-ABRELATEDNAMES" {
			related += len(v)
		}
	}
	if related != 2 {
		t.Errorf("extra = %v, want the base's and the other member's related name, once each", mc.Extra)
	}
}
