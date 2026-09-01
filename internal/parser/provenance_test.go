package parser

import (
	"reflect"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

// TestProvenance: X-ROLODEX-SOURCE written by an earlier run is the record
// of where a card came from; the parser-assigned Source is the fallback.
func TestProvenance(t *testing.T) {
	cases := []struct {
		name string
		c    model.ParsedContact
		want []model.Source
	}{
		{"icloud", model.ParsedContact{Extra: map[string][]string{"X-ROLODEX-SOURCE": {"icloud"}}}, []model.Source{model.SourceICloud}},
		{"google", model.ParsedContact{Extra: map[string][]string{"X-ROLODEX-SOURCE": {"google"}}}, []model.Source{model.SourceGoogle}},
		{"merged", model.ParsedContact{Extra: map[string][]string{"X-ROLODEX-SOURCE": {"merged(icloud+google)"}}}, []model.Source{model.SourceICloud, model.SourceGoogle}},
		{"absent falls back to Source", model.ParsedContact{Source: model.SourceGoogle}, []model.Source{model.SourceGoogle}},
		{"absent on a foreign file", model.ParsedContact{Source: model.SourceUnknown, Extra: map[string][]string{}}, []model.Source{model.SourceUnknown}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Provenance(tc.c); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Provenance() = %v, want %v", got, tc.want)
			}
		})
	}
}

// X-ROLODEX-SOURCE is untrusted on a read-back path (prune reads any .vcf).
// A value rolodex never writes falls back to the parser-assigned source.
func TestProvenanceRejectsValuesRolodexNeverWrites(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []model.Source
	}{
		{"foreign single", "outlook", []model.Source{model.SourceUnknown}},
		{"foreign part of merged", "merged(icloud+evil)", []model.Source{model.SourceUnknown}},
		{"empty", "", []model.Source{model.SourceUnknown}},
		{"whitespace is tolerated", " google ", []model.Source{model.SourceGoogle}},
		{"duplicate parts collapse", "merged(icloud+icloud)", []model.Source{model.SourceICloud}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := model.ParsedContact{Source: model.SourceUnknown, Extra: map[string][]string{"X-ROLODEX-SOURCE": {tc.raw}}}
			if got := Provenance(c); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Provenance(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
