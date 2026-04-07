package writer

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	vcard "github.com/emersion/go-vcard"

	"github.com/fairbearlab/rolodex/internal/model"
)

// WriteFile writes merged contacts to a .vcf file atomically.
// Writes to a temp file first, then renames to prevent partial output on crash.
func WriteFile(path string, contacts []model.MergedContact) error {
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", tmpPath, err)
	}

	if err := Write(f, contacts); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing contacts: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("syncing %s: %w", tmpPath, err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing %s: %w", tmpPath, err)
	}

	// Remove existing file first — os.Rename doesn't overwrite on Windows
	os.Remove(path)
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming %s to %s: %w", tmpPath, path, err)
	}

	return nil
}

// Write writes merged contacts as vCard 3.0 to a writer.
func Write(w io.Writer, contacts []model.MergedContact) error {
	enc := vcard.NewEncoder(w)

	for _, mc := range contacts {
		card := contactToCard(mc)
		if err := enc.Encode(card); err != nil {
			return fmt.Errorf("encoding contact %q: %w", mc.Contact.FormattedName, err)
		}
	}
	return nil
}

func contactToCard(mc model.MergedContact) vcard.Card {
	c := mc.Contact
	card := make(vcard.Card)

	// VERSION
	card.SetValue(vcard.FieldVersion, "3.0")

	// N (structured name)
	nValue := fmt.Sprintf("%s;%s;%s;%s;%s",
		c.FamilyName, c.GivenName, c.MiddleName, c.Prefix, c.Suffix)
	card.Set(vcard.FieldName, &vcard.Field{Value: nValue})

	// FN
	fn := c.FormattedName
	if fn == "" {
		fn = strings.TrimSpace(c.GivenName + " " + c.FamilyName)
	}
	if fn == "" {
		fn = "Unknown"
	}
	card.SetValue(vcard.FieldFormattedName, fn)

	// EMAIL
	for _, e := range c.Emails {
		field := &vcard.Field{Value: e.Address}
		if e.Type != "" {
			field.Params = vcard.Params{"TYPE": {e.Type}}
		}
		card.Add(vcard.FieldEmail, field)
	}

	// TEL
	for _, p := range c.Phones {
		field := &vcard.Field{Value: p.Number}
		if p.Type != "" {
			field.Params = vcard.Params{"TYPE": {p.Type}}
		}
		card.Add(vcard.FieldTelephone, field)
	}

	// ORG
	if c.Org != "" {
		card.SetValue(vcard.FieldOrganization, c.Org)
	}

	// TITLE
	if c.Title != "" {
		card.SetValue(vcard.FieldTitle, c.Title)
	}

	// BDAY
	if c.Birthday != "" {
		card.SetValue(vcard.FieldBirthday, c.Birthday)
	}

	// ADR
	for _, a := range c.Addresses {
		adrValue := fmt.Sprintf("%s;%s;%s;%s;%s;%s;%s",
			a.POBox, a.Extended, a.Street, a.City, a.Region, a.PostCode, a.Country)
		field := &vcard.Field{Value: adrValue}
		if a.Type != "" {
			field.Params = vcard.Params{"TYPE": {a.Type}}
		}
		card.Add(vcard.FieldAddress, field)
	}

	// NOTE
	if c.Note != "" {
		card.SetValue(vcard.FieldNote, c.Note)
	}

	// URL
	if c.URL != "" {
		card.SetValue(vcard.FieldURL, c.URL)
	}

	// PHOTO
	if len(c.Photo) > 0 {
		encoded := base64.StdEncoding.EncodeToString(c.Photo)
		field := &vcard.Field{
			Value: encoded,
			Params: vcard.Params{
				"ENCODING": {"b"},
			},
		}
		if c.PhotoType != "" {
			field.Params["TYPE"] = []string{c.PhotoType}
		}
		card.Add(vcard.FieldPhoto, field)
	}

	// Extra fields (passthrough) — sort keys for deterministic output
	extraKeys := make([]string, 0, len(c.Extra))
	for key := range c.Extra {
		extraKeys = append(extraKeys, key)
	}
	sort.Strings(extraKeys)
	for _, key := range extraKeys {
		for _, v := range c.Extra[key] {
			card.Add(key, &vcard.Field{Value: v})
		}
	}

	// Provenance extension fields
	if len(mc.Sources) > 1 {
		sourceStrs := make([]string, len(mc.Sources))
		for i, s := range mc.Sources {
			sourceStrs[i] = string(s)
		}
		card.SetValue("X-ROLODEX-SOURCE",
			fmt.Sprintf("merged(%s)", strings.Join(sourceStrs, "+")))
	} else if len(mc.Sources) == 1 {
		card.SetValue("X-ROLODEX-SOURCE", string(mc.Sources[0]))
	}

	if mc.Score > 0 {
		card.SetValue("X-ROLODEX-SCORE", fmt.Sprintf("%.2f", mc.Score))
	}

	if mc.ReviewFlag {
		card.SetValue("X-ROLODEX-REVIEW", "true")
	}

	return card
}
