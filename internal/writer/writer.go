package writer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
)

// WriteFile writes merged contacts to a .vcf file atomically.
// Writes to a temp file first, then renames to prevent partial output on crash.
func WriteFile(path string, contacts []model.MergedContact) error {
	tmpPath := filepath.Clean(path + ".tmp")
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("creating %s: %w", tmpPath, err)
	}

	if err := Write(f, contacts); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing contacts: %w", err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("syncing %s: %w", tmpPath, err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing %s: %w", tmpPath, err)
	}

	// Remove existing file first — os.Rename doesn't overwrite on Windows
	_ = os.Remove(path)
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming %s to %s: %w", tmpPath, path, err)
	}

	return nil
}

// Write writes merged contacts as vCard 3.0 to a writer.
//
// Lines are formatted here rather than by go-vcard's encoder. The model
// holds decoded values (family name "O;Brien", note `C:\temp`), so every
// value is escaped on the way out, and a structured value needs `\;` inside
// a component — which that encoder cannot produce: it escapes every
// backslash, so `\;` came out as `\\;`, and the whole-buffer rewrite that
// undid it also turned a genuine backslash before a separator (family
// `Smith\`, given "John") into an escaped semicolon, handing Apple and
// Google one family name "Smith;John". ORG and unmodeled fields are already
// wire form (see model.ParsedContact) and go out as they are. Property
// order matches the old encoder: VERSION first, then by name, insertion
// order within a name.
func Write(w io.Writer, contacts []model.MergedContact) error {
	var buf bytes.Buffer
	for _, mc := range contacts {
		buf.Reset()
		formatCard(&buf, contactProperties(mc))
		if _, err := w.Write(buf.Bytes()); err != nil {
			return fmt.Errorf("writing contact %q: %w", mc.Contact.FormattedName, err)
		}
	}
	return nil
}

// property is one content line: a name, ordered parameters, and a value
// already in wire form.
type property struct {
	name   string
	params [][2]string
	value  string
}

func formatCard(buf *bytes.Buffer, props []property) {
	sort.SliceStable(props, func(i, j int) bool { return props[i].name < props[j].name })
	buf.WriteString("BEGIN:VCARD\r\nVERSION:3.0\r\n")
	for _, p := range props {
		buf.WriteString(p.name)
		for _, kv := range p.params {
			buf.WriteString(";" + kv[0] + "=" + escapeParam(kv[1]))
		}
		buf.WriteString(":" + p.value + "\r\n")
	}
	buf.WriteString("END:VCARD\r\n")
}

// escapeParam formats a parameter value the way go-vcard's encoder did, so
// the output is unchanged for the TYPE and ENCODING parameters we emit.
var paramEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`, ",", `\,`)

func escapeParam(v string) string {
	return paramEscaper.Replace(v)
}

// structured joins decoded components into a wire-form structured value.
func structured(components ...string) string {
	escaped := make([]string, len(components))
	for i, c := range components {
		escaped[i] = normalize.Escape(c)
	}
	return strings.Join(escaped, ";")
}

func contactProperties(mc model.MergedContact) []property {
	c := mc.Contact
	var props []property
	add := func(name, value string, params ...[2]string) {
		props = append(props, property{name: name, params: params, value: value})
	}
	typed := func(t string) [][2]string {
		if t == "" {
			return nil
		}
		return [][2]string{{"TYPE", t}}
	}

	// N (structured name)
	add("N", structured(c.FamilyName, c.GivenName, c.MiddleName, c.Prefix, c.Suffix))

	// FN
	fn := c.FormattedName
	if fn == "" {
		fn = strings.TrimSpace(c.GivenName + " " + c.FamilyName)
	}
	if fn == "" {
		fn = "Unknown"
	}
	add("FN", normalize.Escape(fn))

	// EMAIL
	for _, e := range c.Emails {
		add("EMAIL", normalize.Escape(e.Address), typed(e.Type)...)
	}

	// TEL
	for _, p := range c.Phones {
		add("TEL", normalize.Escape(p.Number), typed(p.Type)...)
	}

	// ORG is kept in wire form by the parser and written back as is.
	if c.Org != "" {
		add("ORG", c.Org)
	}

	// TITLE
	if c.Title != "" {
		add("TITLE", normalize.Escape(c.Title))
	}

	// BDAY
	if c.Birthday != "" {
		add("BDAY", normalize.Escape(c.Birthday))
	}

	// ADR
	for _, a := range c.Addresses {
		add("ADR", structured(a.POBox, a.Extended, a.Street, a.City, a.Region, a.PostCode, a.Country), typed(a.Type)...)
	}

	// NOTE
	if c.Note != "" {
		add("NOTE", normalize.Escape(c.Note))
	}

	// URL
	if c.URL != "" {
		add("URL", normalize.Escape(c.URL))
	}

	// PHOTO
	if len(c.Photo) > 0 {
		params := [][2]string{{"ENCODING", "b"}}
		if c.PhotoType != "" {
			params = append(params, [2]string{"TYPE", c.PhotoType})
		}
		add("PHOTO", base64.StdEncoding.EncodeToString(c.Photo), params...)
	}

	// Extra fields (passthrough, wire form) — sort keys for deterministic output
	extraKeys := make([]string, 0, len(c.Extra))
	for key := range c.Extra {
		extraKeys = append(extraKeys, key)
	}
	sort.Strings(extraKeys)
	for _, key := range extraKeys {
		for _, v := range c.Extra[key] {
			add(key, v)
		}
	}

	// Provenance extension fields
	if len(mc.Sources) > 1 {
		sourceStrs := make([]string, len(mc.Sources))
		for i, s := range mc.Sources {
			sourceStrs[i] = string(s)
		}
		add("X-ROLODEX-SOURCE", fmt.Sprintf("merged(%s)", strings.Join(sourceStrs, "+")))
	} else if len(mc.Sources) == 1 {
		add("X-ROLODEX-SOURCE", string(mc.Sources[0]))
	}

	if mc.Score > 0 {
		add("X-ROLODEX-SCORE", fmt.Sprintf("%.2f", mc.Score))
	}

	if mc.ReviewFlag {
		add("X-ROLODEX-REVIEW", "true")
	}

	return props
}
