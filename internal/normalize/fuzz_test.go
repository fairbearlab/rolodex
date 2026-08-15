package normalize

import "testing"

func FuzzNormalizeName(f *testing.F) {
	f.Add("Robert Smith")
	f.Add("Dr. José María García Jr.")
	f.Add("  ")
	f.Add("")
	f.Add("名前")    // Japanese
	f.Add("Ṡṁíṫḣ") // dotted letters

	f.Fuzz(func(t *testing.T, input string) {
		// Should never panic
		result := Name(input)
		_ = result
	})
}

func FuzzNormalizePhone(f *testing.F) {
	f.Add("+1 (555) 123-4567")
	f.Add("15551234567")
	f.Add("")
	f.Add("abc")
	f.Add("+44 20 7946 0958")

	f.Fuzz(func(t *testing.T, input string) {
		result := Phone(input)
		_ = result
	})
}
