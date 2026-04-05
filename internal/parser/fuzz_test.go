package parser

import (
	"strings"
	"testing"

	"github.com/fairbearlabs/rolodex/internal/model"
)

func FuzzParse(f *testing.F) {
	// Seed with valid vCard data
	f.Add(`BEGIN:VCARD
VERSION:3.0
N:Smith;John;;;
FN:John Smith
END:VCARD`)

	f.Add(`BEGIN:VCARD
VERSION:3.0
N:;;;;;
FN:
EMAIL:test@example.com
END:VCARD`)

	f.Add(`BEGIN:VCARD
VERSION:3.0
FN:No Name Field
TEL:555-1234
END:VCARD`)

	f.Fuzz(func(t *testing.T, input string) {
		// Should never panic, regardless of input
		contacts, _, err := Parse(strings.NewReader(input), model.SourceICloud)
		if err != nil {
			return // errors are fine, panics are not
		}
		_ = contacts
	})
}
