package review

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/fairbearlab/rolodex/internal/model"
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

)

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

func renderCompact(m ReviewModel, c *ReviewCluster) string {
	w := min(m.Width-4, 60)

	// Header
	header := fmt.Sprintf(" Review %d/%d", m.ResolvedCount()+1, len(m.Clusters))
	scoreStr := fmt.Sprintf("Score: %.2f ", c.Decision.Score)
	scoreRendered := scoreHighStyle.Render(scoreStr)
	if c.Decision.Score < CompactThreshold {
		scoreRendered = scoreLowStyle.Render(scoreStr)
	}
	padLen := w - lipgloss.Width(header) - lipgloss.Width(scoreStr)
	if padLen < 1 {
		padLen = 1
	}
	title := titleStyle.Render(header) + strings.Repeat("─", padLen) + scoreRendered

	// Contact names side-by-side
	var lines []string
	if len(c.Contacts) >= 2 {
		a, b := c.Contacts[0], c.Contacts[1]
		nameA := contactDisplayName(a)
		nameB := contactDisplayName(b)
		lines = append(lines, fmt.Sprintf("  %-24s  %s", nameA, nameB))

		// Show matched fields
		if len(a.Emails) > 0 || len(b.Emails) > 0 {
			emailA := firstEmail(a)
			emailB := firstEmail(b)
			lines = append(lines, fmt.Sprintf("  %-24s  %s", emailA, emailB))
		}
		if len(a.Phones) > 0 || len(b.Phones) > 0 {
			phoneA := firstPhone(a)
			phoneB := firstPhone(b)
			lines = append(lines, fmt.Sprintf("  %-24s  %s", phoneA, phoneB))
		}
		if a.Org != "" || b.Org != "" {
			lines = append(lines, fmt.Sprintf("  %-24s  %s", orBlank(a.Org), orBlank(b.Org)))
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
	if len(matchParts) > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+labelStyle.Render("Match: ")+strings.Join(matchParts, ", "))
	}

	// Footer
	lines = append(lines, "")
	lines = append(lines, renderKeybar())

	content := title + "\n\n" + strings.Join(lines, "\n")
	return borderStyle.Width(w).Render(content) + "\n"
}

func renderDetailed(m ReviewModel, c *ReviewCluster) string {
	w := min(m.Width-4, 72)
	cardW := (w - 7) / 2 // two cards with gap

	// Header
	header := fmt.Sprintf(" Review %d/%d", m.ResolvedCount()+1, len(m.Clusters))
	scoreStr := fmt.Sprintf("Score: %.2f ", c.Decision.Score)
	scoreRendered := scoreLowStyle.Render(scoreStr)
	if c.Decision.Score >= CompactThreshold {
		scoreRendered = scoreHighStyle.Render(scoreStr)
	}
	padLen := w - lipgloss.Width(header) - lipgloss.Width(scoreStr)
	if padLen < 1 {
		padLen = 1
	}
	title := titleStyle.Render(header) + strings.Repeat("─", padLen) + scoreRendered

	var body strings.Builder

	if len(c.Contacts) == 2 {
		// Side-by-side cards
		leftCard := renderContactCard(c.Contacts[0], cardW)
		rightCard := renderContactCard(c.Contacts[1], cardW)
		body.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftCard, "  ", rightCard))
	} else {
		// Stacked cards for multi-contact clusters
		for i, contact := range c.Contacts {
			if i > 0 {
				body.WriteString("\n")
			}
			body.WriteString(renderContactCard(contact, w-4))
		}
	}

	// Score breakdown
	body.WriteString("\n\n")
	body.WriteString(renderScoreBreakdown(c))

	// Ambiguity warning
	if c.Decision.Ambiguity != "" {
		body.WriteString("\n")
		body.WriteString(warningStyle.Render("  ! " + truncate(c.Decision.Ambiguity, w-6)))
	}

	// Footer
	body.WriteString("\n\n")
	body.WriteString(renderKeybar())

	content := title + "\n\n" + body.String()

	// Apply scroll offset for long content
	lines := strings.Split(content, "\n")
	maxVisible := m.Height - 6
	if maxVisible < 10 {
		maxVisible = 10
	}
	if len(lines) > maxVisible && m.ScrollOffset > 0 {
		offset := m.ScrollOffset
		if offset > len(lines)-maxVisible {
			offset = len(lines) - maxVisible
		}
		lines = lines[offset:]
	}

	return borderStyle.Width(w).Render(strings.Join(lines, "\n")) + "\n"
}

func renderContactCard(c model.ParsedContact, w int) string {
	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Padding(0, 1).
		Width(w)

	source := string(c.Source)
	if source == "" {
		source = "unknown"
	}
	header := labelStyle.Render("─ " + source + " ")

	var lines []string
	lines = append(lines, header)
	lines = append(lines, field("Name", contactDisplayName(c)))

	if len(c.Emails) > 0 {
		lines = append(lines, field("Email", ""))
		for _, e := range c.Emails {
			lines = append(lines, "  "+e.Address)
		}
	} else {
		lines = append(lines, field("Email", "(none)"))
	}

	if len(c.Phones) > 0 {
		lines = append(lines, field("Phone", ""))
		for _, p := range c.Phones {
			lines = append(lines, "  "+p.Number)
		}
	} else {
		lines = append(lines, field("Phone", "(none)"))
	}

	lines = append(lines, field("Org", orNone(c.Org)))
	lines = append(lines, field("Title", orNone(c.Title)))
	lines = append(lines, field("Birthday", orNone(c.Birthday)))

	if len(c.Addresses) > 0 {
		lines = append(lines, field("Address", ""))
		for _, a := range c.Addresses {
			lines = append(lines, "  "+formatAddress(a))
		}
	}

	return cardStyle.Render(strings.Join(lines, "\n"))
}

func renderScoreBreakdown(c *ReviewCluster) string {
	f := c.Features
	var lines []string
	lines = append(lines, "  "+labelStyle.Render("Score breakdown:"))

	nameLabel := fmt.Sprintf("    Name:  %.2f", f.NameSimilarity)
	lines = append(lines, fmt.Sprintf("%-36s x0.40", nameLabel))

	emailVal := "0.00 (no shared emails)"
	if f.SharedEmail {
		emailVal = "1.00 (shared email)"
	}
	lines = append(lines, fmt.Sprintf("    Email: %-28s x0.25", emailVal))

	phoneVal := "0.00 (no shared phones)"
	if f.SharedPhone {
		phoneVal = "1.00 (shared phone)"
	}
	lines = append(lines, fmt.Sprintf("    Phone: %-28s x0.25", phoneVal))

	orgVal := "0.00 (no shared org)"
	if f.SharedOrg {
		orgVal = "1.00 (shared org)"
	}
	lines = append(lines, fmt.Sprintf("    Org:   %-28s x0.10", orgVal))

	return strings.Join(lines, "\n")
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

	return borderStyle.Width(w).Render(titleStyle.Render(" Help") + "\n\n" + help) + "\n"
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
		return c.Phones[0].Number
	}
	return "(none)"
}

func field(label, value string) string {
	return labelStyle.Render(label+": ") + value
}

func orBlank(s string) string {
	if s == "" {
		return ""
	}
	return s
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
