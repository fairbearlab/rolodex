package scorer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
)

// goldenContacts is a deliberately diverse fixture: every birthday shape
// (full date, no-year, conflicting, unreadable, the 01-01 placeholder,
// empty), every name shape (exact, nickname, near-name, initials, Jr/Sr,
// middle-name variants including the Google-folds-middle-into-given shape,
// diacritics) and nameless contacts with shared identifiers. It exists so
// TestScoreHoistIsEquivalent can pin Score's output across the per-contact
// hoist: the hoist must not change a single Score, Tier or Features value
// for any pair this fixture generates.
func goldenContacts() []model.NormalizedContact {
	raw := []model.ParsedContact{
		{GivenName: "John", FamilyName: "Smith", Birthday: "1989-06-29",
			Emails: []model.Email{{Address: "js@example.com"}}, Phones: []model.Phone{{Number: "5558675309"}}, Org: "Acme"},
		{GivenName: "John", FamilyName: "Smith", Birthday: "1989-06-29",
			Emails: []model.Email{{Address: "js@example.com"}}, Phones: []model.Phone{{Number: "5558675309"}}, Org: "Acme"},
		{GivenName: "John", FamilyName: "Smith", Birthday: "--06-29"},
		{GivenName: "John", FamilyName: "Smith", Birthday: "1990-06-29"},
		{GivenName: "John", FamilyName: "Smith", Birthday: "circa 1950", Phones: []model.Phone{{Number: "3175554444"}}},
		{GivenName: "John", FamilyName: "Smith", Birthday: "1995-12-31", Phones: []model.Phone{{Number: "3175554444"}}},
		{GivenName: "John", FamilyName: "Smith", Birthday: "1970-01-01"},
		{GivenName: "John", FamilyName: "Smith", Birthday: "--01-01"},
		{GivenName: "Bob", FamilyName: "Smith", Emails: []model.Email{{Address: "bob@gmail.com"}}, Phones: []model.Phone{{Number: "5551234567"}}},
		{GivenName: "Robert", FamilyName: "Smith", Emails: []model.Email{{Address: "bob@gmail.com"}}, Phones: []model.Phone{{Number: "5551234567"}}},
		{GivenName: "Chris", FamilyName: "Fielding", Phones: []model.Phone{{Number: "3175559876"}}},
		{GivenName: "Chris", FamilyName: "Fielding", Phones: []model.Phone{{Number: "3175559876"}}, Org: "Continental Aeronautics"},
		{GivenName: "Kris", FamilyName: "Fielding", Phones: []model.Phone{{Number: "5551234567"}}},
		{GivenName: "Eric", FamilyName: "Johnson", Phones: []model.Phone{{Number: "3175551212"}}},
		{GivenName: "Erica", FamilyName: "Johnson", Phones: []model.Phone{{Number: "3175551212"}}},
		{GivenName: "Alex", FamilyName: "", Emails: []model.Email{{Address: "alex.rivera@corp.com"}}, Phones: []model.Phone{{Number: "+1 415 555 0100"}}},
		{GivenName: "Alex", FamilyName: "", Emails: []model.Email{{Address: "alex.tan@corp.com"}}, Phones: []model.Phone{{Number: "(415) 555-0100"}}},
		{GivenName: "Charles", MiddleName: "J.", FamilyName: "Galanti"},
		{GivenName: "Charles", MiddleName: "James", FamilyName: "Galanti"},
		{GivenName: "Charles", MiddleName: "", FamilyName: "Galanti"},
		{GivenName: "John", MiddleName: "", FamilyName: "Smith", Suffix: "Jr."},
		{GivenName: "John", MiddleName: "", FamilyName: "Smith", Suffix: "Sr."},
		{GivenName: "John", MiddleName: "", FamilyName: "Smith Jr.", Suffix: ""},
		{GivenName: "John", MiddleName: "", FamilyName: "Smith Sr.", Suffix: ""},
		{GivenName: "Doe", MiddleName: "V", FamilyName: "", Suffix: ""}, // Google-style given-folds-middle shape below pairs it against
		{GivenName: "John V", MiddleName: "", FamilyName: "Doe"},
		{GivenName: "John", MiddleName: "V", FamilyName: "Doe"},
		{GivenName: "Nguyên", FamilyName: "Le"},
		{GivenName: "Nguyễn", FamilyName: "Le"},
		{GivenName: "", FamilyName: "", Emails: []model.Email{{Address: "shared@example.com"}}, Phones: []model.Phone{{Number: "5551234567"}}},
		{GivenName: "", FamilyName: "", Emails: []model.Email{{Address: "shared@example.com"}}, Phones: []model.Phone{{Number: "5551234567"}}},
		{GivenName: "", FamilyName: "", Emails: []model.Email{{Address: "shared@example.com"}}},
		{GivenName: "Maria", FamilyName: "Rodriguez", Birthday: "1989-13-45", Emails: []model.Email{{Address: "maria.r@example.com"}}},
		{GivenName: "Maria", FamilyName: "Rodriguez", Birthday: "1989-13-45", Emails: []model.Email{{Address: "mrodriguez@example.org"}}},
		{GivenName: "J", FamilyName: "Smith", Phones: []model.Phone{{Number: "3175550001"}}},
		{GivenName: "J.", FamilyName: "Smith", Phones: []model.Phone{{Number: "3175550001"}}},
	}
	contacts := make([]model.NormalizedContact, len(raw))
	for i, p := range raw {
		contacts[i] = normalize.Contact(p)
	}
	return contacts
}

// goldenPairs scores every contact against every other, so the fixture
// exercises the full cross product of birthday, name and identifier shapes.
func goldenPairs(n int) [][2]int {
	var pairs [][2]int
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			pairs = append(pairs, [2]int{i, j})
		}
	}
	return pairs
}

const goldenFixturePath = "testdata/golden_score.json"

// TestScoreHoistIsEquivalent pins Score's output on goldenContacts/goldenPairs
// against a recorded fixture. Regenerate the fixture with
// UPDATE_GOLDEN=1 go test ./internal/scorer/... -run TestScoreHoistIsEquivalent
// only when a deliberate scoring-behavior change accompanies the update; a
// hoist of per-contact work (caching, precomputation) must never need it.
func TestScoreHoistIsEquivalent(t *testing.T) {
	contacts := goldenContacts()
	pairs := goldenPairs(len(contacts))
	got := Score(contacts, pairs)

	gotJSON, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}

	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(goldenFixturePath), 0o750); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenFixturePath, append(gotJSON, '\n'), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		t.Skip("golden fixture regenerated")
	}

	want, err := os.ReadFile(goldenFixturePath)
	if err != nil {
		t.Fatalf("read fixture (run with UPDATE_GOLDEN=1 to create it): %v", err)
	}

	var gotVal, wantVal []model.ScoredPair
	if err := json.Unmarshal(gotJSON, &gotVal); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	if err := json.Unmarshal(want, &wantVal); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if len(gotVal) != len(wantVal) {
		t.Fatalf("len(got) = %d, len(fixture) = %d", len(gotVal), len(wantVal))
	}
	for i := range gotVal {
		g, w := gotVal[i], wantVal[i]
		if g != w {
			t.Errorf("pair %d (%d,%d): got %+v, want %+v", i, g.A, g.B, g, w)
		}
	}
}
