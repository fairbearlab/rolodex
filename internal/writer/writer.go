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

// WriteFile writes merged contacts to a .vcf file atomically: staged beside
// the destination, then renamed into place, so a crash never leaves a
// partial output.
func WriteFile(path string, contacts []model.MergedContact) error {
	s, err := Stage(path, contacts)
	if err != nil {
		return err
	}
	return s.Commit()
}

// Staged is a file written beside its destination but not yet moved into
// place. Commit renames it there; Abort deletes it. A caller writing several
// files stages them all before committing any, so a failure part-way leaves
// every destination exactly as it was.
type Staged struct {
	path string
	tmp  string
}

// Stage writes contacts to a fresh temp file in path's directory, mode
// 0600. The file is created exclusively with a random name, so a symlink or
// a stale file planted at a predictable staging path (the old "<path>.tmp")
// is never opened, let alone written through.
func Stage(path string, contacts []model.MergedContact) (*Staged, error) {
	var buf bytes.Buffer
	if err := Write(&buf, contacts); err != nil {
		return nil, fmt.Errorf("writing contacts: %w", err)
	}
	return StageBytes(path, buf.Bytes())
}

// WriteBytes writes data to path the way WriteFile writes contacts: staged
// beside the destination, fsynced, renamed into place. The JSON reports use
// it so that every file rolodex writes has the same guarantees.
func WriteBytes(path string, data []byte) error {
	s, err := StageBytes(path, data)
	if err != nil {
		return err
	}
	return s.Commit()
}

// StageBytes is Stage for raw bytes.
func StageBytes(path string, data []byte) (*Staged, error) {
	path = filepath.Clean(path)
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		// os.Rename would replace an empty directory with the file.
		return nil, fmt.Errorf("%s is a directory", path)
	}
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	f, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return nil, fmt.Errorf("creating temp file for %s: %w", path, err)
	}
	s := &Staged{path: path, tmp: f.Name()}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		s.Abort()
		return nil, fmt.Errorf("writing %s: %w", s.tmp, err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		s.Abort()
		return nil, fmt.Errorf("syncing %s: %w", s.tmp, err)
	}

	if err := f.Close(); err != nil {
		s.Abort()
		return nil, fmt.Errorf("closing %s: %w", s.tmp, err)
	}
	return s, nil
}

// Commit moves the staged file onto its destination, replacing whatever was
// there. On failure the staged file is discarded and the destination is
// left as it was.
func (s *Staged) Commit() error {
	// os.Rename replaces an existing file on every platform Go supports
	// (MOVEFILE_REPLACE_EXISTING on Windows), so there is no unlink first:
	// the destination is never absent, and an empty directory at the path
	// is refused by Stage instead of being removed here.
	if err := os.Rename(s.tmp, s.path); err != nil {
		s.Abort()
		return fmt.Errorf("renaming %s to %s: %w", s.tmp, s.path, err)
	}
	return nil
}

// Abort discards the staged file without touching the destination. It is
// safe to call after Commit.
func (s *Staged) Abort() {
	_ = os.Remove(s.tmp)
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
	} else if c.PhotoURI != "" {
		params := [][2]string{{"VALUE", "uri"}}
		if c.PhotoType != "" {
			params = append(params, [2]string{"TYPE", c.PhotoType})
		}
		// A URI value is not text: RFC 2426/6350 do not escape it, Apple and
		// Google do not, and it is held in wire form, so it goes out as is.
		add("PHOTO", c.PhotoURI, params...)
	}

	// Extra fields (passthrough, wire form) — sort keys for deterministic output.
	// The writer's own extension fields are regenerated below, so a copy that
	// arrived in Extra from reading back rolodex output is dropped rather
	// than emitted beside the new one: every card resolve wrote used to carry
	// X-ROLODEX-SOURCE twice. SOURCE is always ours to regenerate; SCORE and
	// REVIEW only when there is a new value to emit. CLUSTER is set in Extra
	// by the merger on purpose and passes through.
	skip := map[string]bool{
		"X-ROLODEX-SOURCE": true,
		"X-ROLODEX-SCORE":  mc.Score > 0,
		"X-ROLODEX-REVIEW": mc.ReviewFlag,
	}
	extraKeys := make([]string, 0, len(c.Extra))
	for key := range c.Extra {
		if skip[strings.ToUpper(key)] {
			continue
		}
		extraKeys = append(extraKeys, key)
	}
	sort.Strings(extraKeys)
	for _, key := range extraKeys {
		for _, v := range c.Extra[key] {
			add(key, v)
		}
	}

	// Provenance extension fields. A foreign file (prune reads any .vcf as
	// SourceUnknown) has no provenance to record, so nothing is stamped.
	if len(mc.Sources) > 1 {
		sourceStrs := make([]string, len(mc.Sources))
		for i, s := range mc.Sources {
			sourceStrs[i] = string(s)
		}
		add("X-ROLODEX-SOURCE", fmt.Sprintf("merged(%s)", strings.Join(sourceStrs, "+")))
	} else if len(mc.Sources) == 1 && (mc.Sources[0] == model.SourceICloud || mc.Sources[0] == model.SourceGoogle) {
		// Only a real source is provenance. "unknown" (a foreign file) and
		// the read-back labels "merged"/"review" say where the parser was
		// called from, not where the contact came from.
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
