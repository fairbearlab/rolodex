package review

import (
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

// TestCompactCardShowsTheMatchingIdentifier: every pair in [0.60, 0.78) — a
// near-match name plus one shared identifier — opens in the compact card,
// which showed the FIRST email and phone of each side. Katherine Chen
// (k.chen@work, kchen@personal) and Kathryn Chen (kchen@personal,
// kathryn@other) share their second email, so the card showed two different
// addresses under "Match: name (89%), email", with no indication which one
// matched, no birthday, and no sign of the extra values. The compact card
// must put the identifier that caused the match on both sides, mark it as
// shared, show how many values it is not showing, show birthdays when
// either side has one, and carry a one-line score breakdown.
func TestCompactCardShowsTheMatchingIdentifier(t *testing.T) {
	c := ReviewCluster{
		ClusterID: "kc",
		Decision:  model.ReviewDecision{ClusterID: "kc", Score: 0.60, Decision: "pending"},
		Resolved:  "pending",
		Features:  model.ScoreFeatures{NameSimilarity: 0.89, SharedEmail: true},
		Contacts: []model.ParsedContact{
			{Source: model.SourceICloud, GivenName: "Katherine", FamilyName: "Chen", Birthday: "1989-10-22",
				Emails: []model.Email{{Address: "k.chen@work.example"}, {Address: "kchen@personal.example"}},
				Phones: []model.Phone{{Number: "3175551111"}, {Number: "3175552222"}}},
			{Source: model.SourceGoogle, GivenName: "Kathryn", FamilyName: "Chen",
				Emails: []model.Email{{Address: "kchen@personal.example"}, {Address: "kathryn@other.example"}}},
		},
	}
	m := ReviewModel{Clusters: []ReviewCluster{c}, Width: 100, Height: 40}
	if got := m.ActiveViewMode(); got != ViewCompact {
		t.Fatalf("test precondition: ActiveViewMode = %v, want compact for a 0.60 pair", got)
	}
	out := renderCompact(m, &m.Clusters[0])
	assertBoxFits(t, out, 100)

	if n := strings.Count(out, "kchen@personal.example"); n != 2 {
		t.Errorf("the shared email appears %d times, want it on both sides:\n%s", n, out)
	}
	if strings.Contains(out, "k.chen@work.example") {
		t.Errorf("the card shows the first email instead of the matching one:\n%s", out)
	}
	if !strings.Contains(out, "✓") {
		t.Errorf("the matching email is not marked as shared:\n%s", out)
	}
	if !strings.Contains(out, "+1") {
		t.Errorf("no hint that each side has another email the card is not showing:\n%s", out)
	}
	if !strings.Contains(out, "1989-10-22") {
		t.Errorf("the birthday is not shown:\n%s", out)
	}
	for _, want := range []string{"x0.40", "+0.25"} {
		if !strings.Contains(out, want) {
			t.Errorf("no one-line score breakdown (%q missing):\n%s", want, out)
		}
	}
}
