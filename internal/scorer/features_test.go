package scorer

import (
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

func TestScoreFeaturesPopulated(t *testing.T) {
	a := makeContact("robert", "smith", []string{"bob@gmail.com"}, []string{"5551234567"}, "Acme")
	b := makeContact("robert", "smith", []string{"bob@gmail.com"}, []string{"5551234567"}, "Acme")

	contacts := []model.NormalizedContact{a, b}
	pairs := [][2]int{{0, 1}}
	scored := Score(contacts, pairs)

	f := scored[0].Features
	if f.NameSimilarity < 0.99 {
		t.Errorf("NameSimilarity = %.3f, expected ~1.0 for identical names", f.NameSimilarity)
	}
	if !f.SharedEmail {
		t.Error("SharedEmail should be true")
	}
	if !f.SharedPhone {
		t.Error("SharedPhone should be true")
	}
	if !f.SharedOrg {
		t.Error("SharedOrg should be true")
	}
}

func TestScoreFeaturesNameless(t *testing.T) {
	a := makeContact("", "", []string{"shared@example.com"}, []string{"5551234567"}, "")
	b := makeContact("", "", []string{"shared@example.com"}, []string{"5551234567"}, "")

	contacts := []model.NormalizedContact{a, b}
	pairs := [][2]int{{0, 1}}
	scored := Score(contacts, pairs)

	f := scored[0].Features
	if f.NameSimilarity != 0 {
		t.Errorf("NameSimilarity = %.3f, expected 0 for nameless contacts", f.NameSimilarity)
	}
	if !f.SharedEmail {
		t.Error("SharedEmail should be true")
	}
	if !f.SharedPhone {
		t.Error("SharedPhone should be true")
	}
}

func TestScoreFeaturesPartialMatch(t *testing.T) {
	a := makeContact("alice", "johnson", []string{"alice@example.com"}, nil, "BigCo")
	b := makeContact("alice", "johnson", []string{"other@example.com"}, nil, "SmallCo")

	contacts := []model.NormalizedContact{a, b}
	pairs := [][2]int{{0, 1}}
	scored := Score(contacts, pairs)

	f := scored[0].Features
	if f.NameSimilarity < 0.99 {
		t.Errorf("NameSimilarity = %.3f, expected ~1.0 for identical names", f.NameSimilarity)
	}
	if f.SharedEmail {
		t.Error("SharedEmail should be false for different emails")
	}
	if f.SharedOrg {
		t.Error("SharedOrg should be false for different orgs")
	}
}
