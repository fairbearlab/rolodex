package review

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestModel(scores []float64) ReviewModel {
	report := makeReport(scores)
	contacts := makeContacts(len(scores) * 2)
	clusters := BuildClusters(report, contacts)

	m := ReviewModel{
		Report:   report,
		Clusters: clusters,
		Width:    80,
		Height:   24,
		PairStart: time.Now(),
		StartTime: time.Now(),
	}
	m.AdvanceToNextPending()
	return m
}

func TestDecideMerge(t *testing.T) {
	m := newTestModel([]float64{0.82, 0.65})

	startIdx := m.CurrentIndex
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = result.(ReviewModel)

	// The first cluster should be resolved
	if m.Clusters[startIdx].Resolved != "merge" {
		t.Errorf("cluster resolved = %q, want %q", m.Clusters[startIdx].Resolved, "merge")
	}
	if len(m.Decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(m.Decisions))
	}
	if m.Decisions[0].Choice != "merge" {
		t.Errorf("decision choice = %q, want %q", m.Decisions[0].Choice, "merge")
	}
}

func TestDecideSkip(t *testing.T) {
	m := newTestModel([]float64{0.82, 0.65})

	startIdx := m.CurrentIndex
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = result.(ReviewModel)

	if m.Clusters[startIdx].Resolved != "skip" {
		t.Errorf("cluster resolved = %q, want %q", m.Clusters[startIdx].Resolved, "skip")
	}
}

func TestUndo(t *testing.T) {
	m := newTestModel([]float64{0.82, 0.65})

	// Merge first
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = result.(ReviewModel)

	mergedIdx := m.Decisions[0].ClusterIndex

	// Undo
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = result.(ReviewModel)

	if m.Clusters[mergedIdx].Resolved != "pending" {
		t.Errorf("undone cluster resolved = %q, want %q", m.Clusters[mergedIdx].Resolved, "pending")
	}
	if len(m.Decisions) != 0 {
		t.Errorf("expected 0 decisions after undo, got %d", len(m.Decisions))
	}
	if m.CurrentIndex != mergedIdx {
		t.Errorf("CurrentIndex = %d, want %d (back to undone cluster)", m.CurrentIndex, mergedIdx)
	}
}

func TestUndoEmpty(t *testing.T) {
	m := newTestModel([]float64{0.82})

	// Undo with no decisions should be a no-op
	startIdx := m.CurrentIndex
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = result.(ReviewModel)

	if m.CurrentIndex != startIdx {
		t.Errorf("CurrentIndex changed after empty undo: %d -> %d", startIdx, m.CurrentIndex)
	}
}

func TestToggleView(t *testing.T) {
	m := newTestModel([]float64{0.82}) // compact by default

	if m.ActiveViewMode() != ViewCompact {
		t.Fatal("expected initial compact mode")
	}

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = result.(ReviewModel)

	if m.ActiveViewMode() != ViewDetailed {
		t.Error("expected detailed mode after toggle")
	}

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = result.(ReviewModel)

	if m.ActiveViewMode() != ViewCompact {
		t.Error("expected compact mode after second toggle")
	}
}

func TestHelpToggle(t *testing.T) {
	m := newTestModel([]float64{0.82})

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = result.(ReviewModel)
	if !m.ShowHelp {
		t.Error("expected ShowHelp=true after '?'")
	}

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = result.(ReviewModel)
	if m.ShowHelp {
		t.Error("expected ShowHelp=false after second '?'")
	}
}

func TestAllDecidedDone(t *testing.T) {
	m := newTestModel([]float64{0.82})

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = result.(ReviewModel)

	if !m.Done {
		t.Error("expected Done=true after all clusters decided")
	}
	if cmd == nil {
		t.Error("expected quit command")
	}
}

func TestScrollDetailedView(t *testing.T) {
	m := newTestModel([]float64{0.65}) // detailed mode

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(ReviewModel)

	if m.ScrollOffset != 1 {
		t.Errorf("ScrollOffset = %d, want 1 after 'j'", m.ScrollOffset)
	}

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = result.(ReviewModel)

	if m.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d, want 0 after 'k'", m.ScrollOffset)
	}

	// 'k' at 0 should stay at 0
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = result.(ReviewModel)
	if m.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d, want 0 (no negative scroll)", m.ScrollOffset)
	}
}

// Silence unused import warning
var _ = fmt.Sprintf
