package scorer

import (
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
)

// TestCompatibleMiddlePunctuationOnly is a regression test for a panic:
// a middle name that is punctuation only (".", a common export placeholder)
// trimmed to the empty string, and the single-character branch then sliced
// it, crashing the whole merge run with
// "slice bounds out of range [:1] with length 0".
func TestCompatibleMiddlePunctuationOnly(t *testing.T) {
	tests := []struct {
		name   string
		ma, mb string
		want   bool
	}{
		{"dot vs initial", ".", "j", true},
		{"initial vs dot", "j", ".", true},
		{"double dot vs initial", "..", "x", true},
		{"dot vs full name", ".", "james", true},
		{"dot vs dot", ".", ".", true},
		{"initial matches name", "j", "james", true},
		{"name matches initial", "james", "j", true},
		{"trailing-dot initial matches name", "j.", "james", true},
		{"different names", "james", "john", false},
		{"different initials", "a", "b", false},
		{"absent is compatible", "", "james", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compatibleMiddle(tt.ma, tt.mb)
			if got != tt.want {
				t.Errorf("compatibleMiddle(%q, %q) = %v, want %v", tt.ma, tt.mb, got, tt.want)
			}
		})
	}
}

// TestCompatibleMiddleMultiByteInitial guards the rune-vs-byte comparison:
// a single-rune initial can be multi-byte, and byte-slicing both missed the
// match and split the rune.
func TestCompatibleMiddleMultiByteInitial(t *testing.T) {
	tests := []struct {
		name   string
		ma, mb string
		want   bool
	}{
		{"multibyte initial matches name", "ö", "östen", true},
		{"name matches multibyte initial", "östen", "ö", true},
		{"multibyte initial mismatch", "ö", "anders", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compatibleMiddle(tt.ma, tt.mb)
			if got != tt.want {
				t.Errorf("compatibleMiddle(%q, %q) = %v, want %v", tt.ma, tt.mb, got, tt.want)
			}
		})
	}
}

// TestScorePunctuationMiddleNameDoesNotPanic drives the same input through
// the real Score -> scorePair -> sameName -> compatibleMiddle chain, which
// is how a malformed vCard reached the crash.
func TestScorePunctuationMiddleNameDoesNotPanic(t *testing.T) {
	a := normalize.Contact(model.ParsedContact{
		GivenName: "John", FamilyName: "Smith", MiddleName: ".",
		Phones: []model.Phone{{Number: "555-123-4567"}},
	})
	b := normalize.Contact(model.ParsedContact{
		GivenName: "John", FamilyName: "Smith", MiddleName: "O",
		Phones: []model.Phone{{Number: "555-123-4567"}},
	})

	got := Score([]model.NormalizedContact{a, b}, [][2]int{{0, 1}})
	if len(got) != 1 {
		t.Fatalf("Score returned %d pairs, want 1", len(got))
	}
	// "." carries no information, so the middle names stay compatible and
	// the shared phone confirms the exact-name match.
	if !got[0].Features.NameExact {
		t.Errorf("NameExact = false, want true (a %q middle name should not block the match)", ".")
	}
	if got[0].Tier != model.TierAutoMerge {
		t.Errorf("Tier = %v, want %v", got[0].Tier, model.TierAutoMerge)
	}
}
