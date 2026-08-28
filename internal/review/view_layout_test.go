package review

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/fairbearlab/rolodex/internal/model"
)

func pairCluster() ReviewCluster {
	return ReviewCluster{
		ClusterID: "c1",
		Decision: model.ReviewDecision{
			ClusterID: "c1",
			Score:     0.65,
			Ambiguity: `"Dana Fielding" and "Dana Fielding" scored 0.65 (tier: review)`,
			Decision:  "pending",
		},
		Contacts: []model.ParsedContact{
			{
				Source: model.SourceGoogle, FormattedName: "Dana Fielding",
				GivenName: "Chris", FamilyName: "Fielding",
				Phones:   []model.Phone{{Number: "3175559876"}},
				Org:      "Continental Aeronautics Reserve Command HQ",
				Birthday: "19900101",
				Emails:   []model.Email{{Address: "a.very.long.email.address.for.testing@example-domain.com"}},
			},
			{
				Source: model.SourceICloud, FormattedName: "Dana Fielding",
				GivenName: "Chris", FamilyName: "Fielding",
				Phones: []model.Phone{{Number: "(317) 555-9876"}, {Number: "+1 317 555 0100"}},
				Emails: []model.Email{{Address: "A.Very.Long.Email.Address.For.Testing@example-domain.com"}},
				Addresses: []model.Address{{
					Street: "1234 Some Extremely Long Street Name Boulevard", City: "Indianapolis",
					Region: "IN", PostCode: "46201", Country: "USA",
				}},
			},
		},
		Features: model.ScoreFeatures{NameSimilarity: 1.0, SharedPhone: true, SharedEmail: true},
		Resolved: "pending",
	}
}

// assertBoxFits checks that every rendered line is the same width, that the
// box fits the terminal, and returns the lines for further inspection.
func assertBoxFits(t *testing.T, out string, termWidth int) []string {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	boxW := lipgloss.Width(lines[0])
	if boxW > termWidth {
		t.Errorf("box width %d exceeds terminal width %d", boxW, termWidth)
	}
	for i, l := range lines {
		if lw := lipgloss.Width(l); lw != boxW {
			t.Errorf("line %d width %d != box width %d: %q", i, lw, boxW, l)
		}
	}
	return lines
}

func TestRenderDetailedFitsAtAllWidths(t *testing.T) {
	for _, width := range []int{40, 60, 80, 100, 120, 200} {
		m := ReviewModel{Clusters: []ReviewCluster{pairCluster()}, Width: width, Height: 60}
		lines := assertBoxFits(t, renderDetailed(m, &m.Clusters[0]), width)

		// The two cards must sit side by side: one line holds both top
		// borders, and every line from there to the first bottom border
		// carries both cards (two outer + four card borders). Any wrapping
		// breaks this structure. (Bottom borders may differ: the cards are
		// different heights.)
		top, bottom := -1, -1
		for i, l := range lines {
			switch strings.Count(l, "┌") {
			case 2:
				top = i
			case 1:
				t.Errorf("width %d: card top border wrapped on line %d: %q", width, i, l)
			}
			if bottom == -1 && strings.Contains(l, "└") {
				bottom = i
			}
		}
		if top == -1 || bottom == -1 || bottom <= top {
			t.Fatalf("width %d: could not locate card borders (top=%d bottom=%d)", width, top, bottom)
		}
		for i := top + 1; i < bottom; i++ {
			if n := strings.Count(lines[i], "│"); n != 6 {
				t.Errorf("width %d: line %d has %d card borders, want 6 (wrapped?): %q", width, i, n, lines[i])
			}
		}

		// Header and score share a line; iCloud is on the left and labelled.
		if !strings.Contains(lines[2], "Review 1/1") || !strings.Contains(lines[2], "Score: 0.65") {
			t.Errorf("width %d: title wrapped: %q", width, lines[2])
		}
		labels := lines[top+1]
		if !strings.Contains(labels, "icloud") || !strings.Contains(labels, "google") {
			t.Errorf("width %d: cards not labelled by source: %q", width, labels)
		}
		if strings.Index(labels, "icloud") > strings.Index(labels, "google") {
			t.Errorf("width %d: icloud card should be on the left: %q", width, labels)
		}
	}
}

func TestRenderCompactFitsAtAllWidths(t *testing.T) {
	for _, width := range []int{40, 60, 80, 120, 200} {
		m := ReviewModel{Clusters: []ReviewCluster{pairCluster()}, Width: width, Height: 40}
		lines := assertBoxFits(t, renderCompact(m, &m.Clusters[0]), width)
		if !strings.Contains(lines[2], "Review 1/1") || !strings.Contains(lines[2], "Score: 0.65") {
			t.Errorf("width %d: compact title wrapped: %q", width, lines[2])
		}
	}
}

func TestRenderDetailedWideTerminalUsesSpace(t *testing.T) {
	narrow := ReviewModel{Clusters: []ReviewCluster{pairCluster()}, Width: 80, Height: 60}
	wide := ReviewModel{Clusters: []ReviewCluster{pairCluster()}, Width: 140, Height: 60}
	nw := lipgloss.Width(strings.SplitN(renderDetailed(narrow, &narrow.Clusters[0]), "\n", 2)[0])
	ww := lipgloss.Width(strings.SplitN(renderDetailed(wide, &wide.Clusters[0]), "\n", 2)[0])
	if ww <= nw {
		t.Errorf("wide terminal box (%d) should be wider than narrow (%d)", ww, nw)
	}
}

func TestRenderDetailedMarksSharedValuesAndNormalizesPhones(t *testing.T) {
	m := ReviewModel{Clusters: []ReviewCluster{pairCluster()}, Width: 100, Height: 60}
	out := renderDetailed(m, &m.Clusters[0])
	if strings.Contains(out, "3175559876") {
		t.Error("raw digit-only phone should be shown in canonical form")
	}
	if strings.Count(out, "(317) 555-9876") != 2 {
		t.Errorf("expected canonical phone on both cards:\n%s", out)
	}
	if strings.Count(out, "✓") != 4 { // shared phone + shared email on each card
		t.Errorf("expected 4 match marks, got %d:\n%s", strings.Count(out, "✓"), out)
	}
	if strings.Contains(out, "(317) 555-0100") && !strings.Contains(out, "  (317) 555-0100") {
		t.Error("unshared phone should not carry a match mark")
	}
}

func TestSourceLabelOnlyMarksWinnerAcrossSources(t *testing.T) {
	c := pairCluster()
	m := ReviewModel{Clusters: []ReviewCluster{c}, Width: 100, Height: 60}
	if out := renderDetailed(m, &m.Clusters[0]); strings.Count(out, "wins conflicts") != 1 {
		t.Errorf("mixed pair should mark exactly one winner:\n%s", out)
	}
	if out := renderCompact(m, &m.Clusters[0]); strings.Count(out, "wins conflicts") != 1 {
		t.Errorf("mixed pair (compact) should mark exactly one winner:\n%s", out)
	}

	same := pairCluster()
	same.Contacts[0].Source = model.SourceICloud
	m = ReviewModel{Clusters: []ReviewCluster{same}, Width: 100, Height: 60}
	for _, out := range []string{renderDetailed(m, &m.Clusters[0]), renderCompact(m, &m.Clusters[0])} {
		if strings.Contains(out, "wins conflicts") {
			t.Errorf("same-source pair must not claim a conflict winner:\n%s", out)
		}
		if strings.Count(out, "icloud") != 2 {
			t.Errorf("same-source pair should still label both cards:\n%s", out)
		}
	}
}

func TestProgressCounterUsesCurrentIndex(t *testing.T) {
	clusters := []ReviewCluster{pairCluster(), pairCluster(), pairCluster()}
	clusters[1].Resolved = "merge"
	clusters[2].Resolved = "skip"
	// Two already resolved, but we are looking at the first one: must read 1/3,
	// not ResolvedCount()+1 = 3/3.
	m := ReviewModel{Clusters: clusters, Width: 100, Height: 60, CurrentIndex: 0}
	for _, out := range []string{renderDetailed(m, &m.Clusters[0]), renderCompact(m, &m.Clusters[0])} {
		if !strings.Contains(out, "Review 1/3") {
			t.Errorf("expected 'Review 1/3' in:\n%s", out)
		}
	}
	m.CurrentIndex = 2
	if out := renderDetailed(m, &m.Clusters[2]); !strings.Contains(out, "Review 3/3") {
		t.Errorf("expected 'Review 3/3' in:\n%s", out)
	}
}

func TestScoreBreakdownColumnsAlign(t *testing.T) {
	for _, nameless := range []bool{false, true} {
		c := pairCluster()
		if nameless {
			c.Features.NameSimilarity = 0
			for i := range c.Contacts {
				c.Contacts[i].FormattedName, c.Contacts[i].GivenName, c.Contacts[i].FamilyName = "", "", ""
			}
		}
		col := -1
		for _, l := range strings.Split(renderScoreBreakdown(&c), "\n") {
			idx := strings.Index(l, "x0.")
			if idx < 0 {
				continue
			}
			if col == -1 {
				col = idx
			} else if idx != col {
				t.Errorf("nameless=%v: weight column at %d, want %d: %q", nameless, idx, col, l)
			}
		}
		if col == -1 {
			t.Fatal("no weight rows rendered")
		}
	}
}

func TestDisplayPhone(t *testing.T) {
	cases := map[string]string{
		"3175559876":       "(317) 555-9876",
		"(317) 555-9876":   "(317) 555-9876",
		"+1 317-555-9876":  "(317) 555-9876",
		"+44 20 7946 0958": "+44 20 7946 0958", // not 10 digits: left alone
		"":                 "",
	}
	for in, want := range cases {
		if got := displayPhone(in); got != want {
			t.Errorf("displayPhone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderDetailedShowsSaveError(t *testing.T) {
	m := ReviewModel{Clusters: []ReviewCluster{pairCluster()}, Width: 100, Height: 60}
	m.LastError = errString("disk full")
	if out := renderDetailed(m, &m.Clusters[0]); !strings.Contains(out, "save error: disk full") {
		t.Errorf("save error not rendered:\n%s", out)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestRenderDetailedFitsWideGlyphs(t *testing.T) {
	c := pairCluster()
	c.Contacts[0].FormattedName = "田中太郎田中太郎田中太郎田中太郎田中太郎"
	c.Contacts[0].Org = "株式会社日本電気通信システム研究所;開発部"
	c.Contacts[1].FormattedName = "Zoë 🎉 Ünal-Østergaard Longname Here"
	for _, width := range []int{60, 80, 100} {
		m := ReviewModel{Clusters: []ReviewCluster{c}, Width: width, Height: 60}
		lines := assertBoxFits(t, renderDetailed(m, &m.Clusters[0]), width)
		top, bottom := -1, -1
		for i, l := range lines {
			if strings.Count(l, "┌") == 2 {
				top = i
			}
			if bottom == -1 && strings.Contains(l, "└") {
				bottom = i
			}
		}
		if top == -1 || bottom <= top {
			t.Fatalf("width %d: could not locate cards", width)
		}
		for i := top + 1; i < bottom; i++ {
			if strings.Count(lines[i], "│") != 6 {
				t.Errorf("width %d: wide glyphs wrapped a card line %d: %q", width, i, lines[i])
			}
		}
		out := renderCompact(m, &m.Clusters[0])
		assertBoxFits(t, out, width)
	}
}

func TestTruncateMeasuresDisplayWidth(t *testing.T) {
	if got := truncate("田中太郎", 5); lipgloss.Width(got) > 5 {
		t.Errorf("truncate(CJK, 5) = %q, width %d > 5", got, lipgloss.Width(got))
	}
	if got := truncate("田中太郎", 8); got != "田中太郎" {
		t.Errorf("truncate(CJK, 8) = %q, want unchanged", got)
	}
	if got := truncate("abcdefgh", 6); got != "abc..." {
		t.Errorf("truncate(ascii, 6) = %q, want abc...", got)
	}
	if got := padRight("田中", 6); lipgloss.Width(got) != 6 {
		t.Errorf("padRight(CJK, 6) width = %d, want 6", lipgloss.Width(got))
	}
}

func TestDisplayOrg(t *testing.T) {
	cases := map[string]string{";FRIEND": "FRIEND", "Acme;Sales": "Acme, Sales", "Acme;;Team": "Acme, Team", "Acme": "Acme", "": ""}
	for in, want := range cases {
		if got := displayOrg(in); got != want {
			t.Errorf("displayOrg(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScoreBreakdownUsesScorerNamelessFlag(t *testing.T) {
	// FN-only contact: display name exists, but the scorer used the nameless
	// table (no given name). The breakdown must follow the scorer.
	c := pairCluster()
	c.Features = model.ScoreFeatures{Nameless: true, SharedEmail: true, SharedPhone: true}
	out := renderScoreBreakdown(&c)
	if !strings.Contains(out, "x0.45") || strings.Contains(out, "x0.40") {
		t.Errorf("expected nameless weight table:\n%s", out)
	}

	c.Features = model.ScoreFeatures{NameSimilarity: 1, NameExact: true, SharedPhone: true, BirthdayConflict: true}
	out = renderScoreBreakdown(&c)
	if !strings.Contains(out, "birthdays differ") || !strings.Contains(out, "Held for review") {
		t.Errorf("expected birthday conflict callout:\n%s", out)
	}
}
