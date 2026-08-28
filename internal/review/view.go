package review

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
	"github.com/fairbearlab/rolodex/internal/scorer"
)

// Layout constants. lipgloss semantics that these encode:
//
//   - Style.Width(n) covers content + padding only; a border adds 2 more
//     columns. A box built with Width(n) therefore renders n+2 wide.
//   - borderStyle has Padding(1, 2), so the text inside a Width(n) box is
//     n - outerPad columns wide.
//
// Every width in this file is derived from these so the side-by-side cards
// fit the container exactly instead of hard-wrapping.
const (
	outerPad         = 4 // borderStyle Padding(1, 2): 2 columns each side
	cardBorder       = 2 // NormalBorder on each contact card
	cardGap          = 2 // spacer between the two cards
	minBoxWidth      = 20
	maxDetailedWidth = 120
	maxCompactWidth  = 80
)

var (
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Bold(true)

	scoreHighStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("2")) // green

	scoreLowStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("3")) // yellow

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")) // dim

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3")) // yellow

	keyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("6")) // cyan

	matchStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("2")) // green

	// Per-source card styling. iCloud wins field conflicts on merge, so it
	// gets the stronger colour and an explicit label.
	icloudStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("4")) // blue

	googleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("5")) // magenta
)

// layout returns the outer box width to pass to borderStyle.Width and the
// usable text width inside it, for a terminal termWidth columns wide.
func layout(termWidth, maxWidth int) (box, inner int) {
	box = max(min(termWidth-4, maxWidth), minBoxWidth)
	return box, box - outerPad
}

// cardWidth returns the Width to give each of two side-by-side contact cards
// so that both cards plus the gap fit in inner columns.
func cardWidth(inner int) int {
	return (inner-cardGap)/2 - cardBorder
}

func (m ReviewModel) View() string {
	if m.ShowHelp {
		return renderHelp(m.Width)
	}
	if m.Done {
		return renderSummaryView(m)
	}

	c := m.CurrentCluster()
	if c == nil {
		return renderSummaryView(m)
	}

	switch m.ActiveViewMode() {
	case ViewCompact:
		return renderCompact(m, c)
	default:
		return renderDetailed(m, c)
	}
}

// renderTitle builds the " Review i/n ───── Score: x.xx " rule, sized to the
// inner text width so the score never wraps onto its own line.
func renderTitle(m ReviewModel, c *ReviewCluster, inner int) string {
	header := fmt.Sprintf(" Review %d/%d", m.CurrentIndex+1, len(m.Clusters))
	scoreStr := fmt.Sprintf("Score: %.2f ", c.Decision.Score)
	scoreRendered := scoreLowStyle.Render(scoreStr)
	if c.Decision.Score >= CompactThreshold {
		scoreRendered = scoreHighStyle.Render(scoreStr)
	}
	padLen := inner - lipgloss.Width(header) - lipgloss.Width(scoreStr)
	if padLen < 1 {
		padLen = 1
	}
	return titleStyle.Render(header) + strings.Repeat("─", padLen) + scoreRendered
}

// orderedPair returns the two contacts of a pair with iCloud on the left, so
// the winning side is always in the same place on screen.
func orderedPair(contacts []model.ParsedContact) (model.ParsedContact, model.ParsedContact) {
	a, b := contacts[0], contacts[1]
	if a.Source != model.SourceICloud && b.Source == model.SourceICloud {
		return b, a
	}
	return a, b
}

func renderCompact(m ReviewModel, c *ReviewCluster) string {
	w, inner := layout(m.Width, maxCompactWidth)
	title := renderTitle(m, c, inner)

	// Two columns: left column fixed, right column takes the rest.
	colW := max((inner-4)/2, 10)
	row := func(l, r string) string {
		return "  " + padRight(truncate(l, colW), colW) + "  " + truncate(r, colW)
	}

	// Contact names side-by-side
	var lines []string
	if len(c.Contacts) >= 2 {
		a, b := orderedPair(c.Contacts)
		mixed := a.Source != b.Source
		lines = append(lines, row(sourceLabel(a.Source, mixed), sourceLabel(b.Source, mixed)))
		lines = append(lines, row(contactDisplayName(a), contactDisplayName(b)))

		// Show matched fields
		if len(a.Emails) > 0 || len(b.Emails) > 0 {
			lines = append(lines, row(firstEmail(a), firstEmail(b)))
		}
		if len(a.Phones) > 0 || len(b.Phones) > 0 {
			lines = append(lines, row(firstPhone(a), firstPhone(b)))
		}
		if a.Org != "" || b.Org != "" {
			lines = append(lines, row(displayOrg(a.Org), displayOrg(b.Org)))
		}
	}

	// Match summary
	var matchParts []string
	if c.Features.NameSimilarity > 0 {
		matchParts = append(matchParts, fmt.Sprintf("name (%.0f%%)", c.Features.NameSimilarity*100))
	}
	if c.Features.SharedEmail {
		matchParts = append(matchParts, "email")
	}
	if c.Features.SharedPhone {
		matchParts = append(matchParts, "phone")
	}
	if c.Features.SharedOrg {
		matchParts = append(matchParts, "org")
	}
	if c.Features.SharedBirthday {
		matchParts = append(matchParts, "birthday")
	}
	if len(matchParts) > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+labelStyle.Render("Match: ")+strings.Join(matchParts, ", "))
	}

	// Footer
	lines = append(lines, "")
	lines = append(lines, renderKeybar())

	if m.LastError != nil {
		lines = append(lines, warningStyle.Render("  ! save error: "+m.LastError.Error()))
	}

	content := title + "\n\n" + strings.Join(lines, "\n")
	return borderStyle.Width(w).Render(content) + "\n"
}

func renderDetailed(m ReviewModel, c *ReviewCluster) string {
	w, inner := layout(m.Width, maxDetailedWidth)
	title := renderTitle(m, c, inner)

	var body strings.Builder

	if len(c.Contacts) == 2 {
		// Side-by-side cards, iCloud (the conflict winner) on the left.
		a, b := orderedPair(c.Contacts)
		mixed := a.Source != b.Source
		shared := sharedValues(a, b)
		cardW := cardWidth(inner)
		leftCard := renderContactCard(a, cardW, shared, sourceLabel(a.Source, mixed))
		rightCard := renderContactCard(b, cardW, shared, sourceLabel(b.Source, mixed))
		body.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftCard, strings.Repeat(" ", cardGap), rightCard))
	} else {
		// Stacked cards for multi-contact clusters. The "wins conflicts"
		// label only means something when the cluster spans both sources;
		// blocking pairs same-source duplicates too.
		mixed := hasMixedSources(c.Contacts)
		for i, contact := range c.Contacts {
			if i > 0 {
				body.WriteString("\n")
			}
			body.WriteString(renderContactCard(contact, inner-cardBorder, nil, sourceLabel(contact.Source, mixed)))
		}
	}

	// Score breakdown
	body.WriteString("\n\n")
	body.WriteString(renderScoreBreakdown(c))

	// Ambiguity warning
	if c.Decision.Ambiguity != "" {
		body.WriteString("\n")
		body.WriteString(warningStyle.Render("  ! " + truncate(c.Decision.Ambiguity, inner-4)))
	}

	// Footer
	body.WriteString("\n\n")
	body.WriteString(renderKeybar())

	if m.LastError != nil {
		body.WriteString("\n")
		body.WriteString(warningStyle.Render("  ! save error: " + m.LastError.Error()))
	}

	content := title + "\n\n" + body.String()

	// Apply scroll offset for long content
	lines := strings.Split(content, "\n")
	maxVisible := m.Height - 6
	if maxVisible < 10 {
		maxVisible = 10
	}
	if len(lines) > maxVisible {
		offset := m.ScrollOffset
		if offset < 0 {
			offset = 0
		}
		if offset > len(lines)-maxVisible {
			offset = len(lines) - maxVisible
		}
		// offset is already clamped to len(lines)-maxVisible above, so this
		// cannot fire; it is kept as a slice-bounds backstop in case the
		// clamp above is ever reordered or relaxed.
		end := offset + maxVisible
		if end > len(lines) {
			end = len(lines)
		}
		lines = lines[offset:end]
	}

	return borderStyle.Width(w).Render(strings.Join(lines, "\n")) + "\n"
}

// sharedValues returns the normalized emails and phones present on both
// contacts, so the cards can mark the values that caused the match.
func sharedValues(a, b model.ParsedContact) map[string]bool {
	na := normalize.Contact(a)
	nb := normalize.Contact(b)
	inB := make(map[string]bool)
	for _, e := range nb.NormalizedEmails {
		inB[e] = true
	}
	for _, p := range nb.NormalizedPhones {
		inB[p] = true
	}
	shared := make(map[string]bool)
	for _, e := range na.NormalizedEmails {
		if inB[e] {
			shared[e] = true
		}
	}
	for _, p := range na.NormalizedPhones {
		if inB[p] {
			shared[p] = true
		}
	}
	return shared
}

// hasMixedSources reports whether a cluster spans more than one source.
func hasMixedSources(contacts []model.ParsedContact) bool {
	for i := 1; i < len(contacts); i++ {
		if contacts[i].Source != contacts[0].Source {
			return true
		}
	}
	return false
}

// sourceLabel names a source for display. When the pair spans sources the
// iCloud side is called out as the one that wins field conflicts on merge;
// for two contacts from the same source that label would be meaningless.
func sourceLabel(s model.Source, mixed bool) string {
	switch {
	case s == model.SourceICloud && mixed:
		return "icloud (wins conflicts)"
	case s == "":
		return "unknown"
	default:
		return string(s)
	}
}

func sourceStyle(s model.Source) lipgloss.Style {
	switch s {
	case model.SourceICloud:
		return icloudStyle
	case model.SourceGoogle:
		return googleStyle
	default:
		return labelStyle
	}
}

// renderContactCard renders one contact in a bordered card w columns wide
// (plus the border) under the given source label. shared marks normalized
// emails/phones present on the other contact; those lines get a ✓ so the
// match cause is visible.
func renderContactCard(c model.ParsedContact, w int, shared map[string]bool, label string) string {
	style := sourceStyle(c.Source)
	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(style.GetForeground()).
		Padding(0, 1).
		Width(w)

	// Values must not exceed the card's text width or lipgloss wraps them.
	textW := w - 2
	mark := func(value, key string) string {
		if shared[key] {
			return matchStyle.Render("✓ ") + truncate(value, textW-2)
		}
		return "  " + truncate(value, textW-2)
	}

	var lines []string
	lines = append(lines, style.Render(truncate("─ "+label+" ", textW)))
	lines = append(lines, field("Name", truncate(contactDisplayName(c), textW-6)))

	if len(c.Emails) > 0 {
		lines = append(lines, field("Email", ""))
		for _, e := range c.Emails {
			lines = append(lines, mark(e.Address, normalize.Email(e.Address)))
		}
	} else {
		lines = append(lines, field("Email", "(none)"))
	}

	if len(c.Phones) > 0 {
		lines = append(lines, field("Phone", ""))
		for _, p := range c.Phones {
			lines = append(lines, mark(displayPhone(p.Number), normalize.Phone(p.Number)))
		}
	} else {
		lines = append(lines, field("Phone", "(none)"))
	}

	lines = append(lines, field("Org", truncate(orNone(displayOrg(c.Org)), textW-5)))
	lines = append(lines, field("Title", truncate(orNone(c.Title), textW-7)))
	lines = append(lines, field("Birthday", truncate(orNone(c.Birthday), textW-10)))

	if len(c.Addresses) > 0 {
		lines = append(lines, field("Address", ""))
		for _, a := range c.Addresses {
			lines = append(lines, "  "+truncate(formatAddress(a), textW-2))
		}
	}

	return cardStyle.Render(strings.Join(lines, "\n"))
}

// displayPhone renders a phone number in one canonical form so the same
// number stored as "(317) 555-9876" and "3175559876" looks identical on
// both cards. Numbers that don't normalize to 10 digits are shown as-is.
func displayPhone(raw string) string {
	digits := normalize.Phone(raw)
	if len(digits) == 10 {
		return fmt.Sprintf("(%s) %s-%s", digits[:3], digits[3:6], digits[6:])
	}
	return raw
}

func renderScoreBreakdown(c *ReviewCluster) string {
	f := c.Features
	// The scorer records which weight table it used; older reports without
	// the flag fall back to the display-name heuristic.
	nameless := f.Nameless || (f.NameSimilarity == 0 && !hasDisplayName(c))

	// One formatter for every row so the weight column lines up.
	row := func(label, value string, weight float64) string {
		return fmt.Sprintf("    %-9s %-28s x%.2f", label+":", value, weight)
	}
	shared := func(hit bool, yes, no string) string {
		if hit {
			return "1.00 (" + yes + ")"
		}
		return "0.00 (" + no + ")"
	}

	var lines []string
	lines = append(lines, "  "+labelStyle.Render("Score breakdown:"))

	emailWeight, phoneWeight, orgWeight, bdayWeight :=
		scorer.WeightEmail, scorer.WeightPhone, scorer.WeightOrg, scorer.WeightBirthday
	if nameless {
		lines = append(lines, "    "+labelStyle.Render("(nameless contacts — name weight redistributed)"))
		emailWeight, phoneWeight, orgWeight, bdayWeight =
			scorer.WeightEmailNoName, scorer.WeightPhoneNoName, scorer.WeightOrgNoName, scorer.WeightBirthdayNoName
	} else {
		lines = append(lines, row("Name", fmt.Sprintf("%.2f", f.NameSimilarity), scorer.WeightName))
	}
	lines = append(lines, row("Email", shared(f.SharedEmail, "shared email", "no shared emails"), emailWeight))
	lines = append(lines, row("Phone", shared(f.SharedPhone, "shared phone", "no shared phones"), phoneWeight))
	lines = append(lines, row("Org", shared(f.SharedOrg, "shared org", "no shared org"), orgWeight))
	bdayVal := shared(f.SharedBirthday, "shared birthday", "no shared birthday")
	if f.BirthdayConflict {
		bdayVal = "0.00 (birthdays differ)"
	}
	lines = append(lines, row("Birthday", bdayVal, bdayWeight))

	// Say why the pair is here when the score alone would not have put it here.
	switch {
	case f.BirthdayConflict:
		lines = append(lines, "    "+warningStyle.Render("Held for review: the two birthdays disagree."))
	case !nameless && f.NearName() && c.Decision.Score < model.ThresholdReview:
		lines = append(lines, "    "+labelStyle.Render("Surfaced because the names match."))
	}

	return strings.Join(lines, "\n")
}

// hasDisplayName returns true if any contact in the cluster has a visible name.
func hasDisplayName(c *ReviewCluster) bool {
	for _, contact := range c.Contacts {
		if contact.FormattedName != "" || contact.GivenName != "" || contact.FamilyName != "" {
			return true
		}
	}
	return false
}

func renderKeybar() string {
	return fmt.Sprintf("       %s merge    %s skip    %s details    %s undo    %s help",
		keyStyle.Render("[m]"),
		keyStyle.Render("[s]"),
		keyStyle.Render("[d]"),
		keyStyle.Render("[u]"),
		keyStyle.Render("[?]"))
}

func renderHelp(width int) string {
	w := min(width-4, 50)
	help := `Keyboard shortcuts:

  m       Merge this cluster
  s       Skip this cluster
  d       Toggle compact/detailed view
  u       Undo last decision
  j/down  Scroll down (detailed view)
  k/up    Scroll up (detailed view)
  ?       Toggle this help
  q       Save and quit`

	return borderStyle.Width(w).Render(titleStyle.Render(" Help")+"\n\n"+help) + "\n"
}

// Helpers

func contactDisplayName(c model.ParsedContact) string {
	if c.FormattedName != "" {
		return c.FormattedName
	}
	name := strings.TrimSpace(c.GivenName + " " + c.FamilyName)
	if name != "" {
		return name
	}
	if len(c.Emails) > 0 {
		return c.Emails[0].Address
	}
	if len(c.Phones) > 0 {
		return c.Phones[0].Number
	}
	return "(unknown)"
}

func firstEmail(c model.ParsedContact) string {
	if len(c.Emails) > 0 {
		return c.Emails[0].Address
	}
	return "(none)"
}

func firstPhone(c model.ParsedContact) string {
	if len(c.Phones) > 0 {
		return displayPhone(c.Phones[0].Number)
	}
	return "(none)"
}

func field(label, value string) string {
	return labelStyle.Render(label+": ") + value
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func formatAddress(a model.Address) string {
	parts := []string{a.Street, a.City, a.Region, a.PostCode, a.Country}
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	if len(nonEmpty) == 0 {
		return "(empty)"
	}
	return strings.Join(nonEmpty, ", ")
}

// displayOrg renders a structured ORG value for a human: units joined with
// ", " and empty positional slots dropped (";Engineering" -> "Engineering").
func displayOrg(org string) string {
	var parts []string
	for _, p := range strings.Split(org, ";") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

// padRight pads s with spaces to width terminal columns (not runes, so
// double-width scripts still line up).
func padRight(s string, width int) string {
	if pad := width - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// truncate limits s to maxLen terminal columns, measuring display width so
// CJK and other double-width glyphs do not overflow the card and wrap.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return ansi.Truncate(s, maxLen, "")
	}
	return ansi.Truncate(s, maxLen, "...")
}
