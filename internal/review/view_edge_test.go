package review

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/fairbearlab/rolodex/internal/model"
)

// TestViewModeString covers both arms of the calibration label.
func TestViewModeString(t *testing.T) {
	if got := ViewCompact.String(); got != "compact" {
		t.Errorf("ViewCompact.String() = %q, want %q", got, "compact")
	}
	if got := ViewDetailed.String(); got != "detailed" {
		t.Errorf("ViewDetailed.String() = %q, want %q", got, "detailed")
	}
	// Any unexpected value falls back to compact rather than rendering junk.
	if got := ViewMode(99).String(); got != "compact" {
		t.Errorf("ViewMode(99).String() = %q, want %q", got, "compact")
	}
}

// TestViewRoutesToTheRightRenderer walks every branch of View(): help wins
// over everything, Done and an exhausted cluster list both fall to the
// summary, and the remaining two route by ActiveViewMode.
func TestViewRoutesToTheRightRenderer(t *testing.T) {
	base := func() ReviewModel {
		return ReviewModel{Clusters: []ReviewCluster{pairCluster()}, Width: 100, Height: 40}
	}

	help := base()
	help.ShowHelp = true
	if out := help.View(); !strings.Contains(out, "Keyboard shortcuts") {
		t.Errorf("ShowHelp did not render the help screen, got:\n%s", out)
	}

	// Help takes precedence even when the session is finished.
	helpDone := base()
	helpDone.ShowHelp = true
	helpDone.Done = true
	if out := helpDone.View(); !strings.Contains(out, "Keyboard shortcuts") {
		t.Error("ShowHelp must win over Done")
	}

	done := base()
	done.Done = true
	doneOut := done.View()
	if strings.Contains(doneOut, "Keyboard shortcuts") {
		t.Error("Done rendered the help screen")
	}
	if strings.Contains(doneOut, "[m] merge") {
		t.Error("Done rendered the decision keybar")
	}

	// CurrentIndex past the end has no current cluster: summary, not a panic.
	exhausted := base()
	exhausted.CurrentIndex = 5
	if got := exhausted.View(); got != doneOut {
		t.Error("an exhausted cluster list should render the same summary as Done")
	}

	compact := base()
	compactMode := ViewCompact
	compact.ViewOverride = &compactMode
	compactOut := compact.View()

	detailed := base()
	detailedMode := ViewDetailed
	detailed.ViewOverride = &detailedMode
	detailedOut := detailed.View()

	if compactOut == detailedOut {
		t.Error("compact and detailed views rendered identically")
	}
	if !strings.Contains(detailedOut, "Score breakdown") {
		t.Error("detailed view is missing the score breakdown")
	}
	if strings.Contains(compactOut, "Score breakdown") {
		t.Error("compact view should not carry the score breakdown")
	}
}

// TestRenderHelpFitsTerminal: the help box is capped at 50 columns and never
// overflows a narrow terminal, and lists every key the keybar advertises.
func TestRenderHelpFitsTerminal(t *testing.T) {
	for _, width := range []int{20, 40, 80, 200} {
		out := renderHelp(width)
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		boxW := lipgloss.Width(lines[0])
		for i, l := range lines {
			if lw := lipgloss.Width(l); lw != boxW {
				t.Errorf("width %d: line %d width %d != box width %d", width, i, lw, boxW)
			}
		}
		if boxW > 54 { // 50 content + border
			t.Errorf("width %d: help box %d columns, want <= 54", width, boxW)
		}
		for _, key := range []string{"m", "s", "d", "u", "j/down", "k/up", "?", "q"} {
			if !strings.Contains(out, key) {
				t.Errorf("width %d: help is missing key %q", width, key)
			}
		}
	}
}

// TestSourceLabelUnknownSource: an unset source reads as "unknown" rather
// than an empty label, and only iCloud on a mixed pair is called the winner.
func TestSourceLabelUnknownSource(t *testing.T) {
	cases := []struct {
		src   model.Source
		mixed bool
		want  string
	}{
		{model.SourceICloud, true, "icloud (wins conflicts)"},
		{model.SourceICloud, false, "icloud"},
		{model.SourceGoogle, true, "google"},
		{model.SourceGoogle, false, "google"},
		{"", true, "unknown"},
		{"", false, "unknown"},
		{model.SourceUnknown, false, "unknown"},
	}
	for _, tc := range cases {
		if got := sourceLabel(tc.src, tc.mixed); got != tc.want {
			t.Errorf("sourceLabel(%q, %v) = %q, want %q", tc.src, tc.mixed, got, tc.want)
		}
	}
}

// TestSourceStyleDistinctPerSource: the two sources must be visually
// distinguishable, and an unknown source falls back to the dim label style.
func TestSourceStyleDistinctPerSource(t *testing.T) {
	ic := sourceStyle(model.SourceICloud).GetForeground()
	gg := sourceStyle(model.SourceGoogle).GetForeground()
	unk := sourceStyle("").GetForeground()
	if ic == gg {
		t.Error("iCloud and Google cards share a colour")
	}
	if unk != labelStyle.GetForeground() {
		t.Errorf("unknown source colour = %v, want the dim label colour %v", unk, labelStyle.GetForeground())
	}
	if unk == ic || unk == gg {
		t.Error("the unknown-source colour collides with a known source")
	}
}

// TestFirstEmailAndPhoneFallBackToNone covers the empty-slice arms, and that
// firstPhone renders through the canonical phone formatter.
func TestFirstEmailAndPhoneFallBackToNone(t *testing.T) {
	empty := model.ParsedContact{}
	if got := firstEmail(empty); got != "(none)" {
		t.Errorf("firstEmail(empty) = %q, want %q", got, "(none)")
	}
	if got := firstPhone(empty); got != "(none)" {
		t.Errorf("firstPhone(empty) = %q, want %q", got, "(none)")
	}

	full := model.ParsedContact{
		Emails: []model.Email{{Address: "a@b.com"}, {Address: "c@d.com"}},
		Phones: []model.Phone{{Number: "3175559876"}, {Number: "5551234567"}},
	}
	if got := firstEmail(full); got != "a@b.com" {
		t.Errorf("firstEmail = %q, want the first address", got)
	}
	if got := firstPhone(full); got != "(317) 555-9876" {
		t.Errorf("firstPhone = %q, want the first number in canonical form", got)
	}
}

// TestTruncateShortBudgets covers the maxLen <= 0 and maxLen <= 3 arms, where
// there is no room for an ellipsis.
func TestTruncateShortBudgets(t *testing.T) {
	cases := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"hello", 0, ""},
		{"hello", -5, ""},
		{"hello", 1, "h"},
		{"hello", 3, "hel"},
		{"hello", 4, "h..."},
		{"hello", 5, "hello"},
		{"hello", 99, "hello"},
		{"", 5, ""},
	}
	for _, tc := range cases {
		got := truncate(tc.s, tc.maxLen)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.s, tc.maxLen, got, tc.want)
		}
		if tc.maxLen > 0 && lipgloss.Width(got) > tc.maxLen {
			t.Errorf("truncate(%q, %d) = %q is %d columns, over budget", tc.s, tc.maxLen, got, lipgloss.Width(got))
		}
	}
}

// TestPadRightUsesDisplayWidth: padding is measured in terminal columns so
// double-width glyphs still line the second column up.
func TestPadRightUsesDisplayWidth(t *testing.T) {
	if got := lipgloss.Width(padRight("ab", 6)); got != 6 {
		t.Errorf("padRight(\"ab\", 6) is %d columns, want 6", got)
	}
	if got := lipgloss.Width(padRight("日本語", 10)); got != 10 {
		t.Errorf("padRight on CJK is %d columns, want 10", got)
	}
	// Already at or over budget: returned unchanged, never truncated.
	if got := padRight("abcdef", 3); got != "abcdef" {
		t.Errorf("padRight over budget = %q, want the input unchanged", got)
	}
}

// TestHasDisplayNameFalseForNamelessCluster covers both arms: any one named
// contact is enough, and a cluster of pure identifiers has no display name.
func TestHasDisplayNameFalseForNamelessCluster(t *testing.T) {
	nameless := &ReviewCluster{Contacts: []model.ParsedContact{
		{Emails: []model.Email{{Address: "a@b.com"}}},
		{Phones: []model.Phone{{Number: "5551234567"}}},
	}}
	if hasDisplayName(nameless) {
		t.Error("hasDisplayName = true for a cluster with no name fields")
	}
	if hasDisplayName(&ReviewCluster{}) {
		t.Error("hasDisplayName = true for an empty cluster")
	}
	for _, named := range []model.ParsedContact{
		{FormattedName: "A B"}, {GivenName: "A"}, {FamilyName: "B"},
	} {
		c := &ReviewCluster{Contacts: []model.ParsedContact{{}, named}}
		if !hasDisplayName(c) {
			t.Errorf("hasDisplayName = false for a cluster containing %+v", named)
		}
	}
}

// TestRenderDetailedStacksMultiContactClusters: three-way clusters use the
// stacked layout (no side-by-side cards) and still fit the terminal.
func TestRenderDetailedStacksMultiContactClusters(t *testing.T) {
	c := pairCluster()
	c.Contacts = append(c.Contacts, model.ParsedContact{
		Source: model.SourceGoogle, FormattedName: "Dana Fielding",
		GivenName: "Chris", FamilyName: "Fielding",
		Emails: []model.Email{{Address: "third@example.com"}},
	})
	for _, width := range []int{40, 80, 120} {
		m := ReviewModel{Clusters: []ReviewCluster{c}, Width: width, Height: 200}
		out := renderDetailed(m, &m.Clusters[0])
		lines := assertBoxFits(t, out, width)
		if !strings.Contains(out, "third@example.com") {
			t.Errorf("width %d: third contact missing from the stacked view", width)
		}
		// Stacked cards mean no line carries two card top-borders.
		for i, l := range lines {
			if strings.Count(l, "┌") > 1 {
				t.Errorf("width %d: line %d has side-by-side cards in a 3-contact cluster: %q", width, i, l)
			}
		}
	}
	// A >2 cluster is forced detailed regardless of score.
	c.Decision.Score = 0.99
	m := ReviewModel{Clusters: []ReviewCluster{c}, Width: 100, Height: 40}
	if got := m.ActiveViewMode(); got != ViewDetailed {
		t.Errorf("ActiveViewMode for a 3-contact cluster = %v, want detailed", got)
	}
}

// TestRenderDetailedClampsScrollOffset: an out-of-range offset (rapid j/k on
// a short terminal) must clamp rather than slice out of bounds, and the last
// page must stay full height.
func TestRenderDetailedClampsScrollOffset(t *testing.T) {
	c := pairCluster()
	for _, offset := range []int{-100, -1, 0, 3, 500, 1 << 20} {
		m := ReviewModel{Clusters: []ReviewCluster{c}, Width: 80, Height: 16, ScrollOffset: offset}
		out := renderDetailed(m, &m.Clusters[0])
		lines := assertBoxFits(t, out, 80)
		if len(lines) < 3 {
			t.Errorf("offset %d: rendered only %d lines", offset, len(lines))
		}
	}

	// Every offset past the end renders the same final page.
	end := func(off int) string {
		m := ReviewModel{Clusters: []ReviewCluster{c}, Width: 80, Height: 16, ScrollOffset: off}
		return renderDetailed(m, &m.Clusters[0])
	}
	if end(500) != end(1<<20) {
		t.Error("offsets past the end render different pages; the clamp is not stable")
	}
	if end(-100) != end(0) {
		t.Error("a negative offset should clamp to the top of the content")
	}
}

// TestRenderDetailedTinyHeightKeepsMinimumWindow: Height 0 (before the first
// WindowSizeMsg) must not produce a negative window.
func TestRenderDetailedTinyHeightKeepsMinimumWindow(t *testing.T) {
	c := pairCluster()
	for _, h := range []int{0, 1, 5, 6, 12} {
		m := ReviewModel{Clusters: []ReviewCluster{c}, Width: 80, Height: h}
		lines := assertBoxFits(t, renderDetailed(m, &m.Clusters[0]), 80)
		// borderStyle adds 2 border + 2 padding rows around a >=10 line window.
		if len(lines) < 10 {
			t.Errorf("height %d: rendered %d lines, want at least the 10-line minimum window", h, len(lines))
		}
	}
}

// TestRenderCompactWithFewerThanTwoContacts: a degenerate cluster still
// renders a usable frame instead of panicking on contacts[1].
func TestRenderCompactWithFewerThanTwoContacts(t *testing.T) {
	for _, contacts := range [][]model.ParsedContact{
		nil,
		{{Source: model.SourceICloud, FormattedName: "Solo Contact"}},
	} {
		c := pairCluster()
		c.Contacts = contacts
		m := ReviewModel{Clusters: []ReviewCluster{c}, Width: 80, Height: 40}
		out := renderCompact(m, &m.Clusters[0])
		assertBoxFits(t, out, 80)
		if !strings.Contains(out, "Review 1/1") {
			t.Errorf("degenerate cluster lost its title bar:\n%s", out)
		}
		if !strings.Contains(out, "merge") {
			t.Errorf("degenerate cluster lost its keybar:\n%s", out)
		}
	}
}

// TestRenderCompactShowsSaveError mirrors the detailed-view case: a failed
// report write must be visible in compact mode too, not silently swallowed.
func TestRenderCompactShowsSaveError(t *testing.T) {
	m := ReviewModel{
		Clusters:  []ReviewCluster{pairCluster()},
		Width:     80,
		Height:    40,
		LastError: errors.New("permission denied"),
	}
	out := renderCompact(m, &m.Clusters[0])
	assertBoxFits(t, out, 80)
	if !strings.Contains(out, "save error") || !strings.Contains(out, "permission denied") {
		t.Errorf("compact view did not surface the save error:\n%s", out)
	}

	m.LastError = nil
	if strings.Contains(renderCompact(m, &m.Clusters[0]), "save error") {
		t.Error("compact view showed a save error with no error set")
	}
}

// TestCurrentClusterNilWhenExhausted covers both arms of the bounds guard.
func TestCurrentClusterNilWhenExhausted(t *testing.T) {
	m := ReviewModel{Clusters: []ReviewCluster{pairCluster(), pairCluster()}}
	if m.CurrentCluster() == nil {
		t.Fatal("CurrentCluster() = nil at index 0")
	}
	m.CurrentIndex = 1
	if m.CurrentCluster() == nil {
		t.Fatal("CurrentCluster() = nil at the last index")
	}
	m.CurrentIndex = 2
	if m.CurrentCluster() != nil {
		t.Error("CurrentCluster() != nil past the end")
	}
	empty := ReviewModel{}
	if empty.CurrentCluster() != nil {
		t.Error("CurrentCluster() != nil for an empty cluster list")
	}
	// ActiveViewMode must tolerate the nil cluster rather than dereference it.
	if got := empty.ActiveViewMode(); got != ViewCompact {
		t.Errorf("ActiveViewMode() with no cluster = %v, want compact", got)
	}
}

// TestAdvanceToNextPendingWrapsAndExhausts covers the wrap-around scan and
// the all-resolved exit, which is what ends the session.
func TestAdvanceToNextPendingWrapsAndExhausts(t *testing.T) {
	mk := func(resolved ...string) *ReviewModel {
		m := &ReviewModel{}
		for _, r := range resolved {
			c := pairCluster()
			c.Resolved = r
			m.Clusters = append(m.Clusters, c)
		}
		return m
	}

	// Forward scan finds a later pending cluster.
	m := mk("merge", "pending", "skip")
	if !m.AdvanceToNextPending() || m.CurrentIndex != 1 {
		t.Errorf("forward scan: advanced=%v index=%d, want true/1", m.AdvanceToNextPending(), m.CurrentIndex)
	}

	// Nothing pending at or after the cursor: wrap to the front.
	m = mk("pending", "merge", "skip")
	m.CurrentIndex = 2
	m.ScrollOffset = 7
	override := ViewDetailed
	m.ViewOverride = &override
	if !m.AdvanceToNextPending() {
		t.Fatal("wrap-around scan returned false with a pending cluster at index 0")
	}
	if m.CurrentIndex != 0 {
		t.Errorf("wrapped to index %d, want 0", m.CurrentIndex)
	}
	if m.ViewOverride != nil {
		t.Error("ViewOverride survived the move to a new cluster")
	}
	if m.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d after moving, want 0", m.ScrollOffset)
	}
	if m.PairStart.IsZero() {
		t.Error("PairStart was not restarted for the new cluster")
	}

	// All resolved: no move, and the caller learns the session is over.
	m = mk("merge", "skip")
	m.CurrentIndex = 1
	if m.AdvanceToNextPending() {
		t.Error("AdvanceToNextPending() = true with nothing pending")
	}
	if m.CurrentIndex != 1 {
		t.Errorf("CurrentIndex moved to %d when nothing was pending", m.CurrentIndex)
	}

	// Empty cluster list.
	if (&ReviewModel{}).AdvanceToNextPending() {
		t.Error("AdvanceToNextPending() = true on an empty cluster list")
	}
}

// TestActiveViewModeOverrideBeatsEverything: the 'd' toggle is absolute, even
// over the multi-contact and birthday-conflict rules that force detailed.
func TestActiveViewModeOverrideBeatsEverything(t *testing.T) {
	c := pairCluster()
	c.Features.BirthdayConflict = true
	c.Contacts = append(c.Contacts, model.ParsedContact{FormattedName: "Third"})
	m := ReviewModel{Clusters: []ReviewCluster{c}}
	if got := m.ActiveViewMode(); got != ViewDetailed {
		t.Fatalf("ActiveViewMode = %v without an override, want detailed", got)
	}
	compact := ViewCompact
	m.ViewOverride = &compact
	if got := m.ActiveViewMode(); got != ViewCompact {
		t.Errorf("ActiveViewMode = %v with a compact override, want compact", got)
	}
}

// TestActiveViewModeCompactThresholdBoundary pins the >= at CompactThreshold.
func TestActiveViewModeCompactThresholdBoundary(t *testing.T) {
	cases := []struct {
		score float64
		want  ViewMode
	}{
		{CompactThreshold, ViewCompact},
		{CompactThreshold + 0.01, ViewCompact},
		{CompactThreshold - 0.01, ViewDetailed},
		{0, ViewDetailed},
		{1.0, ViewCompact},
	}
	for _, tc := range cases {
		c := pairCluster()
		c.Decision.Score = tc.score
		m := ReviewModel{Clusters: []ReviewCluster{c}}
		if got := m.ActiveViewMode(); got != tc.want {
			t.Errorf("score %.2f: ActiveViewMode = %v, want %v", tc.score, got, tc.want)
		}
	}
}

// TestOrderedPairPutsICloudLeft: the conflict winner is always on the left so
// the eye does not have to hunt for it between cards.
func TestOrderedPairPutsICloudLeft(t *testing.T) {
	ic := model.ParsedContact{Source: model.SourceICloud, FormattedName: "I"}
	gg := model.ParsedContact{Source: model.SourceGoogle, FormattedName: "G"}
	unk := model.ParsedContact{FormattedName: "U"}

	if a, _ := orderedPair([]model.ParsedContact{gg, ic}); a.FormattedName != "I" {
		t.Errorf("google-first pair put %q on the left, want the iCloud card", a.FormattedName)
	}
	if a, _ := orderedPair([]model.ParsedContact{ic, gg}); a.FormattedName != "I" {
		t.Errorf("iCloud-first pair put %q on the left", a.FormattedName)
	}
	// No iCloud side: original order is preserved.
	if a, b := orderedPair([]model.ParsedContact{gg, unk}); a.FormattedName != "G" || b.FormattedName != "U" {
		t.Errorf("non-iCloud pair reordered to %q/%q, want G/U", a.FormattedName, b.FormattedName)
	}
	// Two iCloud cards: no swap.
	ic2 := model.ParsedContact{Source: model.SourceICloud, FormattedName: "I2"}
	if a, _ := orderedPair([]model.ParsedContact{ic, ic2}); a.FormattedName != "I" {
		t.Error("two iCloud cards were reordered")
	}
}

// TestCardWidthNeverCollapses: at every terminal width the two side-by-side
// cards plus the gap must fit the inner text area with room for content.
func TestCardWidthNeverCollapses(t *testing.T) {
	for width := 0; width <= 200; width++ {
		box, inner := layout(width, maxDetailedWidth)
		if box < minBoxWidth {
			t.Fatalf("width %d: box %d below the %d minimum", width, box, minBoxWidth)
		}
		if inner != box-outerPad {
			t.Fatalf("width %d: inner %d != box %d - %d", width, inner, box, outerPad)
		}
		cw := cardWidth(inner)
		if cw <= 0 {
			t.Fatalf("width %d: card width %d is not renderable", width, cw)
		}
		// Both cards plus their borders and the gap must fit the inner area.
		if total := 2*(cw+cardBorder) + cardGap; total > inner {
			t.Fatalf("width %d: two cards need %d columns but inner is %d", width, total, inner)
		}
	}
}

// TestRenderTitleNeverWraps: the score must stay on the header rule at every
// width, including 1-cluster and 4-digit cluster counts.
func TestRenderTitleNeverWraps(t *testing.T) {
	c := pairCluster()
	for _, width := range []int{20, 40, 80, 120, 200} {
		for _, n := range []int{1, 10, 1000} {
			m := ReviewModel{Clusters: make([]ReviewCluster, n), Width: width, Height: 40}
			_, inner := layout(width, maxDetailedWidth)
			title := renderTitle(m, &c, inner)
			if strings.Contains(title, "\n") {
				t.Errorf("width %d n %d: title wrapped:\n%s", width, n, title)
			}
			if !strings.Contains(title, "Score:") {
				t.Errorf("width %d n %d: title lost the score", width, n)
			}
			if !strings.Contains(title, "1/") {
				t.Errorf("width %d n %d: title lost the progress counter", width, n)
			}
		}
	}
}

// TestOrNoneAndFieldRendering covers the small formatters the cards lean on.
func TestOrNoneAndFieldRendering(t *testing.T) {
	if got := orNone(""); got != "(none)" {
		t.Errorf("orNone(\"\") = %q, want %q", got, "(none)")
	}
	if got := orNone("Acme"); got != "Acme" {
		t.Errorf("orNone(%q) = %q, want it unchanged", "Acme", got)
	}
	if got := field("Name", "Chris"); !strings.Contains(got, "Name:") || !strings.Contains(got, "Chris") {
		t.Errorf("field() = %q, want a %q label and the value", got, "Name:")
	}
}

// TestHasMixedSources: the "wins conflicts" label is only meaningful when the
// cluster actually spans both exports. Blocking pairs same-source duplicates
// too, and labelling one of those the winner would be a lie.
func TestHasMixedSources(t *testing.T) {
	ic := model.ParsedContact{Source: model.SourceICloud}
	gg := model.ParsedContact{Source: model.SourceGoogle}
	unk := model.ParsedContact{}

	cases := []struct {
		name     string
		contacts []model.ParsedContact
		want     bool
	}{
		{"nil", nil, false},
		{"single contact", []model.ParsedContact{ic}, false},
		{"two iCloud", []model.ParsedContact{ic, ic}, false},
		{"two Google", []model.ParsedContact{gg, gg}, false},
		{"iCloud + Google", []model.ParsedContact{ic, gg}, true},
		{"Google + iCloud", []model.ParsedContact{gg, ic}, true},
		{"three same source", []model.ParsedContact{gg, gg, gg}, false},
		{"three, last differs", []model.ParsedContact{gg, gg, ic}, true},
		{"known + unknown", []model.ParsedContact{ic, unk}, true},
		{"all unknown", []model.ParsedContact{unk, unk}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasMixedSources(tc.contacts); got != tc.want {
				t.Errorf("hasMixedSources = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestStackedCardsOnlyClaimAWinnerAcrossSources: a same-source multi-contact
// cluster must not label any card "wins conflicts".
func TestStackedCardsOnlyClaimAWinnerAcrossSources(t *testing.T) {
	mk := func(sources ...model.Source) ReviewCluster {
		c := pairCluster()
		c.Contacts = nil
		for i, s := range sources {
			c.Contacts = append(c.Contacts, model.ParsedContact{
				Source: s, FormattedName: "Dana Fielding", GivenName: "Chris", FamilyName: "Fielding",
				Emails: []model.Email{{Address: string(rune('a'+i)) + "@example.com"}},
			})
		}
		return c
	}

	sameSource := mk(model.SourceICloud, model.SourceICloud, model.SourceICloud)
	m := ReviewModel{Clusters: []ReviewCluster{sameSource}, Width: 100, Height: 200}
	if out := renderDetailed(m, &m.Clusters[0]); strings.Contains(out, "wins conflicts") {
		t.Errorf("same-source cluster claimed a conflict winner:\n%s", out)
	}

	mixed := mk(model.SourceICloud, model.SourceGoogle, model.SourceGoogle)
	m = ReviewModel{Clusters: []ReviewCluster{mixed}, Width: 100, Height: 200}
	out := renderDetailed(m, &m.Clusters[0])
	if !strings.Contains(out, "wins conflicts") {
		t.Errorf("mixed-source cluster did not mark the iCloud winner:\n%s", out)
	}
	if n := strings.Count(out, "wins conflicts"); n != 1 {
		t.Errorf("cluster marked %d winners, want exactly 1", n)
	}
}

// TestCompactMatchSummaryListsEverySignal covers the org and birthday chips,
// which only appear when the scorer set those features.
func TestCompactMatchSummaryListsEverySignal(t *testing.T) {
	c := pairCluster()
	c.Features = model.ScoreFeatures{
		NameSimilarity: 1.0, SharedEmail: true, SharedPhone: true,
		SharedOrg: true, SharedBirthday: true,
	}
	m := ReviewModel{Clusters: []ReviewCluster{c}, Width: 100, Height: 40}
	out := renderCompact(m, &m.Clusters[0])
	assertBoxFits(t, out, 100)
	for _, want := range []string{"Match:", "name (100%)", "email", "phone", "org", "birthday"} {
		if !strings.Contains(out, want) {
			t.Errorf("compact match summary missing %q:\n%s", want, out)
		}
	}

	// No signals at all: the Match line is omitted rather than left dangling.
	c.Features = model.ScoreFeatures{}
	m = ReviewModel{Clusters: []ReviewCluster{c}, Width: 100, Height: 40}
	if out := renderCompact(m, &m.Clusters[0]); strings.Contains(out, "Match:") {
		t.Errorf("compact view rendered an empty Match line:\n%s", out)
	}
}

// TestScoreBreakdownExplainsNearNameSurfacing: a pair that only the near-name
// rule put in review must say so, or the user sees a low score with no reason.
func TestScoreBreakdownExplainsNearNameSurfacing(t *testing.T) {
	c := pairCluster()
	c.Decision.Score = model.ThresholdReview - 0.1
	c.Features = model.ScoreFeatures{NameSimilarity: 1.0}
	out := renderScoreBreakdown(&c)
	if !strings.Contains(out, "Surfaced because the names match") {
		t.Errorf("near-name pair did not explain why it is in review:\n%s", out)
	}

	// Above the review threshold the score speaks for itself.
	c.Decision.Score = model.ThresholdReview + 0.1
	if out := renderScoreBreakdown(&c); strings.Contains(out, "Surfaced because") {
		t.Errorf("a pair above the review threshold claimed near-name surfacing:\n%s", out)
	}

	// A birthday conflict is the louder reason and wins the explanation slot.
	c.Decision.Score = model.ThresholdReview - 0.1
	c.Features.BirthdayConflict = true
	out = renderScoreBreakdown(&c)
	if !strings.Contains(out, "birthdays disagree") {
		t.Errorf("birthday conflict not explained:\n%s", out)
	}
	if strings.Contains(out, "Surfaced because") {
		t.Error("birthday conflict and near-name explanations both rendered")
	}
	if !strings.Contains(out, "birthdays differ") {
		t.Errorf("birthday row did not show the conflict:\n%s", out)
	}
}

// A contact with no email shows "(none)" in the compact card rather than an
// empty column.
func TestCompactEmailNone(t *testing.T) {
	got := compactEmail(model.ParsedContact{}, map[string]bool{})
	if !strings.Contains(got, "(none)") {
		t.Errorf("compactEmail with no addresses = %q, want (none)", got)
	}
}

// A nameless pair's breakdown uses the redistributed no-name weights and
// carries no name term — the number shown must be the number the scorer used.
func TestCompactBreakdownNameless(t *testing.T) {
	c := pairCluster()
	c.Features.Nameless = true
	c.Features.NameSimilarity = 0
	out := compactBreakdown(&c)
	if strings.Contains(out, "name ") {
		t.Errorf("nameless breakdown shows a name term:\n%s", out)
	}
	if !strings.Contains(out, "email") || !strings.Contains(out, "phone") {
		t.Errorf("breakdown lacks the shared signals:\n%s", out)
	}
}
