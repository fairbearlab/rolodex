package review

import (
	"time"

	"github.com/charmbracelet/bubbletea"

	"github.com/fairbearlab/rolodex/internal/calibration"
)

func (m ReviewModel) Init() tea.Cmd {
	return nil
}

func (m ReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m ReviewModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Help toggle works everywhere
	if key == "?" {
		m.ShowHelp = !m.ShowHelp
		return m, nil
	}

	// If help is showing, any other key dismisses it
	if m.ShowHelp {
		m.ShowHelp = false
		return m, nil
	}

	// Quit
	if key == "q" || key == "ctrl+c" {
		m.Done = true
		return m, tea.Quit
	}

	// If already done (summary screen), any key quits
	if m.Done || m.CurrentCluster() == nil {
		return m, tea.Quit
	}

	switch key {
	case "m":
		return m.decide("merge")
	case "s":
		return m.decide("skip")
	case "d":
		return m.toggleView()
	case "u":
		return m.undo()
	case "j", "down":
		m.ScrollOffset++
		return m, nil
	case "k", "up":
		if m.ScrollOffset > 0 {
			m.ScrollOffset--
		}
		return m, nil
	}

	return m, nil
}

func (m ReviewModel) decide(choice string) (tea.Model, tea.Cmd) {
	c := m.CurrentCluster()
	if c == nil {
		return m, nil
	}

	elapsed := time.Since(m.PairStart).Milliseconds()

	// Record the decision
	c.Resolved = choice
	decision := UserDecision{
		ClusterIndex: m.CurrentIndex,
		Choice:       choice,
		Timestamp:    time.Now(),
		DecisionMs:   elapsed,
		ViewMode:     m.ActiveViewMode(),
	}
	m.Decisions = append(m.Decisions, decision)

	// Log calibration entry
	if m.CalLog != nil {
		if err := m.CalLog.Append(calibration.Entry{
			ClusterID:      c.ClusterID,
			Decision:       choice,
			Score:          c.Decision.Score,
			Features:       c.Features,
			ViewMode:       decision.ViewMode.String(),
			DecisionTimeMs: elapsed,
			Timestamp:      time.Now(),
		}); err != nil {
			m.LastError = err
		}
	}

	// Persist to report.json
	m.persistReport()

	// Advance to next pending
	if !m.AdvanceToNextPending() {
		m.Done = true
		return m, tea.Quit
	}

	return m, nil
}

func (m ReviewModel) toggleView() (tea.Model, tea.Cmd) {
	current := m.ActiveViewMode()
	if current == ViewCompact {
		v := ViewDetailed
		m.ViewOverride = &v
	} else {
		v := ViewCompact
		m.ViewOverride = &v
	}
	m.ScrollOffset = 0
	return m, nil
}

func (m ReviewModel) undo() (tea.Model, tea.Cmd) {
	if len(m.Decisions) == 0 {
		return m, nil
	}

	// Pop last decision
	last := m.Decisions[len(m.Decisions)-1]
	m.Decisions = m.Decisions[:len(m.Decisions)-1]

	// Revert the cluster
	m.Clusters[last.ClusterIndex].Resolved = "pending"
	m.CurrentIndex = last.ClusterIndex
	m.ViewOverride = nil
	m.ScrollOffset = 0
	m.PairStart = time.Now()

	// Log undo in calibration
	if m.CalLog != nil {
		c := &m.Clusters[last.ClusterIndex]
		if err := m.CalLog.Append(calibration.Entry{
			ClusterID:      c.ClusterID,
			Decision:       "undo",
			Score:          c.Decision.Score,
			Features:       c.Features,
			ViewMode:       last.ViewMode.String(),
			DecisionTimeMs: 0,
			Timestamp:      time.Now(),
		}); err != nil {
			m.LastError = err
		}
	}

	// Persist the undo to report.json
	m.persistReport()

	return m, nil
}

// persistReport writes the current decision state back to report.json.
func (m *ReviewModel) persistReport() {
	if m.ReportPath == "" {
		return
	}

	// Update report.Review decisions from clusters.
	// Clusters were reordered (sorted by score), so we need to match by ClusterID.
	clusterDecisions := make(map[string]string)
	for _, c := range m.Clusters {
		clusterDecisions[c.ClusterID] = c.Resolved
	}

	for i, rd := range m.Report.Review {
		if decision, ok := clusterDecisions[rd.ClusterID]; ok {
			m.Report.Review[i].Decision = decision
		}
	}

	// Persist report atomically
	if err := writeReport(m.ReportPath, m.Report); err != nil {
		m.LastError = err
	}
}
