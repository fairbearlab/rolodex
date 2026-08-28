package model

import "testing"

// TestNearName pins the boundary of the near-name floor: it is the rule that
// keeps same-name pairs out of the distinct bucket, so the >= at
// ThresholdNearName is load-bearing.
func TestNearName(t *testing.T) {
	cases := []struct {
		name string
		sim  float64
		want bool
	}{
		{"identical", 1.0, true},
		{"exactly at threshold", ThresholdNearName, true},
		{"just below threshold", ThresholdNearName - 0.0001, false},
		{"just above threshold", ThresholdNearName + 0.0001, true},
		{"unrelated names", 0.4, false},
		{"zero (nameless pair)", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := ScoreFeatures{NameSimilarity: tc.sim}
			if got := f.NearName(); got != tc.want {
				t.Errorf("NearName() with similarity %.4f = %v, want %v", tc.sim, got, tc.want)
			}
		})
	}
}

// TestThresholdOrdering guards the invariant the tier rules assume:
// distinct < review < auto_merge, and the near-name floor sits above the
// auto-merge score threshold (it is a name rule, not a score rule).
func TestThresholdOrdering(t *testing.T) {
	if !(ThresholdReview < ThresholdAutoMerge) {
		t.Errorf("ThresholdReview (%.2f) must be below ThresholdAutoMerge (%.2f)", ThresholdReview, ThresholdAutoMerge)
	}
	if !(ThresholdNearName > ThresholdAutoMerge) {
		t.Errorf("ThresholdNearName (%.2f) must be above ThresholdAutoMerge (%.2f)", ThresholdNearName, ThresholdAutoMerge)
	}
	if ThresholdNearName > 1.0 {
		t.Errorf("ThresholdNearName (%.2f) must be a reachable similarity", ThresholdNearName)
	}
}
