package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/parser"
)

// pruneFixture is four cards: two reachable (email; phone), one address-only,
// one with nothing but a name, org and photo.
const pruneFixture = "BEGIN:VCARD\r\nVERSION:3.0\r\nN:Able;Ann;;;\r\nFN:Ann Able\r\nEMAIL;TYPE=HOME:ann@example.com\r\nEND:VCARD\r\n" +
	"BEGIN:VCARD\r\nVERSION:3.0\r\nN:Baker;Bob;;;\r\nFN:Bob Baker\r\nTEL;TYPE=CELL:415-555-0102\r\nEND:VCARD\r\n" +
	"BEGIN:VCARD\r\nVERSION:3.0\r\nN:Cole;Cat;;;\r\nFN:Cat Cole\r\nADR;TYPE=HOME:;;1 Main St;Denver;CO;80202;US\r\nEND:VCARD\r\n" +
	"BEGIN:VCARD\r\nVERSION:3.0\r\nN:Dunn;Dee;;;\r\nFN:Dee Dunn\r\nORG:Acme\r\nPHOTO;ENCODING=b;TYPE=JPEG:/9j/4A==\r\nEND:VCARD\r\n"

func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(p))
	if err != nil {
		t.Fatalf("reading %s: %v", p, err)
	}
	return string(b)
}

func countCards(t *testing.T, p string) int {
	t.Helper()
	return strings.Count(readFile(t, p), "BEGIN:VCARD")
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestPruneDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", pruneFixture)

	var stdout bytes.Buffer
	if err := runPrune([]string{in}, &stdout); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if got := dirEntries(t, dir); len(got) != 1 {
		t.Errorf("dry run created files: %v", got)
	}
	out := stdout.String()
	for _, want := range []string{
		"Contact prune (dry run)",
		"Total contacts: 4",
		"Reachable by email, phone, or address: 3",
		"Unreachable: 1",
		"  with org or title: 1",
		"  with URL: 0",
		"  with birthday: 0",
		"  name only: 0",
		"  1. Dee Dunn — has: org, photo",
		"1 contacts would be removed. Re-run with --out kept.vcf to write kept.vcf and removed.vcf.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
}

func TestPruneOutWritesBothFiles(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", pruneFixture)
	kept := filepath.Join(dir, "kept.vcf")
	removed := filepath.Join(dir, "removed.vcf")

	var stdout bytes.Buffer
	if err := runPrune([]string{in, "--out", kept}, &stdout); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if k, r := countCards(t, kept), countCards(t, removed); k != 3 || r != 1 || k+r != 4 {
		t.Errorf("kept=%d removed=%d, want 3+1=4", k, r)
	}
	if !strings.Contains(readFile(t, removed), "FN:Dee Dunn") || strings.Contains(readFile(t, kept), "FN:Dee Dunn") {
		t.Error("Dee Dunn is not in removed.vcf (or is also in kept.vcf)")
	}
	out := stdout.String()
	for _, want := range []string{
		"Contact prune\n",
		"Wrote 3 contacts -> " + kept,
		"Wrote 1 contacts -> " + removed,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "dry run") || strings.Contains(out, "would be removed") {
		t.Errorf("write run still talks like a dry run:\n%s", out)
	}
}

func TestPruneRemovedFlagOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", pruneFixture)
	kept := filepath.Join(dir, "kept.vcf")
	archive := filepath.Join(dir, "archive", "unreachable.vcf")
	if err := os.Mkdir(filepath.Dir(archive), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runPrune([]string{in, "--out", kept, "--removed", archive}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if countCards(t, archive) != 1 {
		t.Errorf("removed set not written to --removed path")
	}
	if _, err := os.Stat(filepath.Join(dir, "removed.vcf")); err == nil {
		t.Error("default removed.vcf written although --removed was given")
	}
}

func TestPruneRejectsCollidingPaths(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", pruneFixture)
	kept := filepath.Join(dir, "kept.vcf")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"out and removed same", []string{in, "--out", kept, "--removed", kept}, "--out and --removed refer to the same file"},
		{"out over input", []string{in, "--out", in}, "<file> and --out refer to the same file"},
		{"removed over input", []string{in, "--out", kept, "--removed", in}, "<file> and --removed refer to the same file"},
		{"out and removed differ by case", []string{in, "--out", filepath.Join(dir, "X.VCF"), "--removed", filepath.Join(dir, "x.vcf")}, "refer to the same file"},
		{"removed without out", []string{in, "--removed", filepath.Join(dir, "r.vcf")}, "--removed requires --out"},
		{"unknown channel", []string{in, "--out", kept, "--reachable-by", "fax"}, `unknown channel "fax"`},
		{"empty channels", []string{in, "--out", kept, "--reachable-by", ""}, "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "out and removed differ by case" {
				// Only a case-insensitive filesystem makes these one file.
				probe := filepath.Join(dir, "Probe")
				if err := os.WriteFile(probe, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(dir, "probe")); err != nil {
					t.Skip("case-sensitive filesystem")
				}
			}
			err := runPrune(tc.args, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
			if got := dirEntries(t, dir); len(got) > 2 {
				t.Errorf("files written despite the error: %v", got)
			}
			if got := readFile(t, in); got != pruneFixture {
				t.Error("input was modified")
			}
		})
	}
	// The valid channel spellings are named so the user can fix the typo.
	err := runPrune([]string{in, "--reachable-by", "fax"}, &bytes.Buffer{})
	for _, ch := range []string{"email", "phone", "address", "url"} {
		if err == nil || !strings.Contains(err.Error(), ch) {
			t.Errorf("error %v does not name %s", err, ch)
		}
	}
}

func TestPruneReachableByChangesTheSplit(t *testing.T) {
	dir := t.TempDir()
	withURL := pruneFixture + "BEGIN:VCARD\r\nVERSION:3.0\r\nN:Ellis;Eve;;;\r\nFN:Eve Ellis\r\nURL:https://facebook.com/eve\r\nEND:VCARD\r\n"
	in := writeFixture(t, dir, "in.vcf", withURL)
	kept := filepath.Join(dir, "kept.vcf")
	removed := filepath.Join(dir, "removed.vcf")

	// address off: Cat Cole (address only) joins the removed set
	var stdout bytes.Buffer
	if err := runPrune([]string{in, "--out", kept, "--reachable-by", "email,phone"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if k, r := countCards(t, kept), countCards(t, removed); k != 2 || r != 3 {
		t.Errorf("email,phone: kept=%d removed=%d, want 2/3", k, r)
	}
	if !strings.Contains(stdout.String(), "Reachable by email or phone: 2") {
		t.Errorf("report does not name the enabled channels:\n%s", stdout.String())
	}
	if !strings.Contains(readFile(t, removed), "FN:Cat Cole") {
		t.Error("address-only contact not in removed.vcf with address disabled")
	}

	// url on: Eve Ellis is kept
	if err := runPrune([]string{in, "--out", kept, "--reachable-by", "email,phone,address,url"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if k, r := countCards(t, kept), countCards(t, removed); k != 4 || r != 1 {
		t.Errorf("with url: kept=%d removed=%d, want 4/1", k, r)
	}
	if !strings.Contains(readFile(t, kept), "FN:Eve Ellis") {
		t.Error("URL-only contact not kept with url enabled")
	}
}

func TestPruneEmptyRemovedReplacesStaleFile(t *testing.T) {
	dir := t.TempDir()
	allReachable := "BEGIN:VCARD\r\nVERSION:3.0\r\nN:Able;Ann;;;\r\nFN:Ann Able\r\nEMAIL:ann@example.com\r\nEND:VCARD\r\n"
	in := writeFixture(t, dir, "in.vcf", allReachable)
	kept := filepath.Join(dir, "kept.vcf")
	removed := writeFixture(t, dir, "removed.vcf", pruneFixture) // stale, from an earlier run

	if err := runPrune([]string{in, "--out", kept}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, removed); got != "" {
		t.Errorf("removed.vcf should be empty, has %d bytes:\n%s", len(got), got)
	}
	if countCards(t, kept) != 1 {
		t.Error("kept.vcf lacks the one contact")
	}
}

// If kept.vcf cannot be written after removed.vcf was, removed.vcf is
// removed again: no run leaves exactly one of the two files.
func TestPruneRollsBackRemovedWhenKeptFails(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", pruneFixture)
	kept := filepath.Join(dir, "no-such-dir", "kept.vcf")
	removed := filepath.Join(dir, "removed.vcf")

	err := runPrune([]string{in, "--out", kept, "--removed", removed}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected an error writing into a missing directory")
	}
	if _, statErr := os.Stat(removed); statErr == nil {
		t.Error("removed.vcf survived a failed kept.vcf write")
	}
}

const malformedFixture = pruneFixture + "BEGIN:VCARD\r\nVERSION:3.0\r\nN:Chen;Kath"

func TestPruneMalformedDryRunReports(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", malformedFixture)
	var stdout bytes.Buffer
	var err error
	stderr := captureStderr(t, func() { err = runPrune([]string{in}, &stdout) })
	if err != nil {
		t.Fatalf("dry run should only report: %v", err)
	}
	if !strings.Contains(stderr, "warning: 1 malformed entries in "+in) {
		t.Errorf("stderr lacks the warning:\n%s", stderr)
	}
	if !strings.Contains(stdout.String(), "Total contacts: 4") {
		t.Errorf("total should count parsed contacts only:\n%s", stdout.String())
	}
	if got := dirEntries(t, dir); len(got) != 1 {
		t.Errorf("dry run created files: %v", got)
	}
}

func TestPruneMalformedOutRefusesUnlessSkipped(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", malformedFixture)
	kept := filepath.Join(dir, "kept.vcf")

	var err error
	captureStderr(t, func() { err = runPrune([]string{in, "--out", kept}, &bytes.Buffer{}) })
	want := "1 malformed entries would be in neither output; fix the file or pass --skip-malformed"
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
	if got := dirEntries(t, dir); len(got) != 1 {
		t.Errorf("files written despite refusing: %v", got)
	}

	stderr := captureStderr(t, func() { err = runPrune([]string{in, "--out", kept, "--skip-malformed"}, &bytes.Buffer{}) })
	if err != nil {
		t.Fatalf("--skip-malformed: %v", err)
	}
	if !strings.Contains(stderr, "warning: 1 malformed entries") {
		t.Errorf("--skip-malformed must still warn:\n%s", stderr)
	}
	if k, r := countCards(t, kept), countCards(t, filepath.Join(dir, "removed.vcf")); k != 3 || r != 1 {
		t.Errorf("kept=%d removed=%d, want 3/1", k, r)
	}
}

func TestPruneJSONShape(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", pruneFixture)
	kept := filepath.Join(dir, "kept.vcf")

	decode := func(args []string) map[string]any {
		var stdout bytes.Buffer
		if err := runPrune(args, &stdout); err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
		}
		return got
	}

	dry := decode([]string{in, "--format", "json"})
	if dry["total"] != 4.0 || dry["kept"] != 3.0 || dry["removed"] != 1.0 {
		t.Errorf("counts = %v/%v/%v", dry["total"], dry["kept"], dry["removed"])
	}
	if dry["dry_run"] != true || dry["out"] != "" || dry["removed_path"] != "" {
		t.Errorf("dry run fields wrong: dry_run=%v out=%q removed_path=%q", dry["dry_run"], dry["out"], dry["removed_path"])
	}
	if by, _ := dry["reachable_by"].([]any); len(by) != 3 || by[0] != "email" || by[2] != "address" {
		t.Errorf("reachable_by = %v", dry["reachable_by"])
	}
	rc, _ := dry["removed_contacts"].([]any)
	if len(rc) != 1 {
		t.Fatalf("removed_contacts = %v, want one entry", dry["removed_contacts"])
	}
	entry, _ := rc[0].(map[string]any)
	for k, want := range map[string]any{
		"name": "Dee Dunn", "index": 3.0, "has_org": true, "has_title": false, "has_address": false,
		"has_url": false, "has_birthday": false, "has_photo": true,
	} {
		if entry[k] != want {
			t.Errorf("removed_contacts[0].%s = %v, want %v", k, entry[k], want)
		}
	}
	if dry["warning_count"] != 0.0 {
		t.Errorf("warning_count = %v", dry["warning_count"])
	}
	if w, ok := dry["warnings"].([]any); !ok || len(w) != 0 {
		t.Errorf("warnings = %v, want []", dry["warnings"])
	}

	written := decode([]string{in, "--format", "json", "--out", kept})
	if written["dry_run"] != false || written["out"] != kept || written["removed_path"] != filepath.Join(dir, "removed.vcf") {
		t.Errorf("after write: dry_run=%v out=%q removed_path=%q", written["dry_run"], written["out"], written["removed_path"])
	}
	if countCards(t, kept) != 3 {
		t.Error("--format json --out did not write kept.vcf")
	}

	// removed_contacts is [] even when nothing is removed; warnings lists parse warnings.
	allOK := writeFixture(t, dir, "ok.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:A\r\nEMAIL:a@b.co\r\nEND:VCARD\r\nBEGIN:VCARD\r\nVERSION:3.0\r\nN:Chen;Kath")
	var empty map[string]any
	if stderr := captureStderr(t, func() { empty = decode([]string{allOK, "--format", "json"}) }); strings.Contains(stderr, "warning:") {
		t.Errorf("JSON mode prints the text warning beside the warnings field:\n%s", stderr)
	}
	if rc, ok := empty["removed_contacts"].([]any); !ok || len(rc) != 0 {
		t.Errorf("removed_contacts = %v, want []", empty["removed_contacts"])
	}
	if empty["warning_count"] != 1.0 {
		t.Errorf("warning_count = %v, want 1", empty["warning_count"])
	}
	if w, _ := empty["warnings"].([]any); len(w) != 1 {
		t.Errorf("warnings = %v, want one entry", empty["warnings"])
	}
}

// pruneOutputs runs prune --out on in and returns the two parsed outputs.
func pruneOutputs(t *testing.T, in string) (kept, removed []model.ParsedContact, keptPath, removedPath string) {
	t.Helper()
	dir := filepath.Dir(in)
	keptPath = filepath.Join(dir, "kept.vcf")
	removedPath = filepath.Join(dir, "removed.vcf")
	if err := runPrune([]string{in, "--out", keptPath}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prune: %v", err)
	}
	var err error
	kept, _, err = parser.ParseFile(keptPath, model.SourceUnknown)
	if err != nil {
		t.Fatal(err)
	}
	removed, _, err = parser.ParseFile(removedPath, model.SourceUnknown)
	if err != nil {
		t.Fatal(err)
	}
	return kept, removed, keptPath, removedPath
}

// A card that carried provenance keeps exactly one line with the same value;
// a card from a foreign export gets none. Before, the writer stamped
// X-ROLODEX-SOURCE:unknown on every foreign card and doubled the line on
// every rolodex-written one.
func TestPrunePreservesProvenanceOnce(t *testing.T) {
	dir := t.TempDir()
	fixture := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:From Google\r\nEMAIL:g@example.com\r\nX-ROLODEX-SOURCE:google\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Merged\r\nTEL:415-555-0101\r\nX-ROLODEX-SOURCE:merged(icloud+google)\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Foreign\r\nEMAIL:f@example.com\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Removed Google\r\nORG:Acme\r\nX-ROLODEX-SOURCE:google\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Removed Foreign\r\nORG:Acme\r\nEND:VCARD\r\n"
	in := writeFixture(t, dir, "in.vcf", fixture)
	_, _, keptPath, removedPath := pruneOutputs(t, in)

	for _, tc := range []struct {
		path string
		want map[string]string // FN -> expected X-ROLODEX-SOURCE line ("" = none)
	}{
		{keptPath, map[string]string{"From Google": "X-ROLODEX-SOURCE:google", "Merged": "X-ROLODEX-SOURCE:merged(icloud+google)", "Foreign": ""}},
		{removedPath, map[string]string{"Removed Google": "X-ROLODEX-SOURCE:google", "Removed Foreign": ""}},
	} {
		cards := strings.Split(readFile(t, tc.path), "END:VCARD")
		seen := 0
		for _, card := range cards {
			for fn, wantLine := range tc.want {
				if !strings.Contains(card, "FN:"+fn+"\r\n") {
					continue
				}
				seen++
				n := strings.Count(card, "X-ROLODEX-SOURCE")
				switch {
				case wantLine == "" && n != 0:
					t.Errorf("%s: foreign card %q was stamped:\n%s", filepath.Base(tc.path), fn, card)
				case wantLine != "" && (n != 1 || !strings.Contains(card, wantLine+"\r\n")):
					t.Errorf("%s: card %q has %d provenance lines, want exactly %q:\n%s", filepath.Base(tc.path), fn, n, wantLine, card)
				}
			}
		}
		if seen != len(tc.want) {
			t.Errorf("%s: found %d of %d expected cards", filepath.Base(tc.path), seen, len(tc.want))
		}
	}
}

// Regression: merge then resolve wrote X-ROLODEX-SOURCE twice on every card
// (the read-back copy in Extra plus the writer's own), and prune of that
// output would have made it three.
func TestMergeResolveThenPruneWriteProvenanceOnce(t *testing.T) {
	dir := t.TempDir()
	merged := filepath.Join(dir, "merged.vcf")
	review := filepath.Join(dir, "review.vcf")
	report := filepath.Join(dir, "report.json")
	final := filepath.Join(dir, "final.vcf")

	// Alice auto-merges on an exact name and shared email; the two David
	// Lees are a near-name review pair, left pending so resolve keeps both.
	card := func(given, family, email, tel string) string {
		return "BEGIN:VCARD\r\nVERSION:3.0\r\nN:" + family + ";" + given + ";;;\r\nFN:" + given + " " + family +
			"\r\nEMAIL:" + email + "\r\nTEL:" + tel + "\r\nEND:VCARD\r\n"
	}
	icloud := writeFixture(t, dir, "icloud.vcf", card("Alice", "Ng", "alice@example.com", "415-555-0001")+card("David", "Lee", "d1@example.com", "317-555-0001"))
	google := writeFixture(t, dir, "google.vcf", card("Alice", "Ng", "alice@example.com", "415-555-0002")+card("David", "Lee", "d2@example.com", "415-555-0003"))
	if err := merge(icloud, google, merged, review, report, false); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if _, err := os.Stat(review); err != nil {
		t.Fatalf("fixture produced no review pair: %v", err)
	}
	if err := resolve(report, review, merged, final); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	assertOneSourcePerCard := func(path string) {
		t.Helper()
		data := readFile(t, path)
		cards, lines := strings.Count(data, "BEGIN:VCARD"), strings.Count(data, "X-ROLODEX-SOURCE:")
		if cards == 0 || lines != cards {
			t.Errorf("%s: %d cards but %d X-ROLODEX-SOURCE lines", filepath.Base(path), cards, lines)
		}
	}
	assertOneSourcePerCard(final)

	_, _, keptPath, _ := pruneOutputs(t, final)
	assertOneSourcePerCard(keptPath)
	if !strings.Contains(readFile(t, keptPath), "X-ROLODEX-SOURCE:merged(icloud+google)") {
		t.Error("merged provenance value not preserved through prune")
	}
}

// Every modeled property and every unmodeled one is present in the output
// with the same value: prune must not lose a field of a contact it keeps.
func TestPruneRoundTripsEveryField(t *testing.T) {
	dir := t.TempDir()
	fixture := "BEGIN:VCARD\r\nVERSION:3.0\r\nN:O\\;Brien;Seán;M.;Dr.;Jr.\r\nFN:Dr. Seán O;Brien Jr.\r\n" +
		"EMAIL;TYPE=HOME:sean@example.com\r\nEMAIL;TYPE=WORK:s.obrien@work.example\r\n" +
		"TEL;TYPE=CELL:+1 (415) 555-0101\r\nTEL;TYPE=HOME:415-555-0102\r\n" +
		"ORG:Acme\\; Inc.;R&D\r\nTITLE:CTO\r\nBDAY:1980-05-06\r\n" +
		"ADR;TYPE=HOME:PO 1;Suite 2;1 Main St;Denver;CO;80202;US\r\n" +
		"NOTE:Line one\\nC:\\\\temp\\, done\r\nURL:https://example.com/~sean\r\n" +
		"PHOTO;ENCODING=b;TYPE=JPEG:/9j/4AAQSkZJRg==\r\n" +
		"CATEGORIES:friends,work\r\nX-FOO:bar\r\nX-FOO:baz\r\nX-ROLODEX-SOURCE:icloud\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nN:Dunn;Dee;;;\r\nFN:Dee Dunn\r\nORG:Acme\r\nTITLE:CEO\r\nBDAY:--1022\r\n" +
		"URL:https://facebook.com/dee\r\nNOTE:Met at a conference\r\nPHOTO;ENCODING=b;TYPE=PNG:iVBORw0KGgo=\r\n" +
		"CATEGORIES:myContacts\r\nX-ABUID:ABC-123\r\nEND:VCARD\r\n"
	in := writeFixture(t, dir, "in.vcf", fixture)
	input, warnings, err := parser.ParseFile(in, model.SourceUnknown)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("fixture does not parse cleanly: %v %v", err, warnings)
	}
	kept, removed, _, _ := pruneOutputs(t, in)
	if len(kept) != 1 || len(removed) != 1 {
		t.Fatalf("kept=%d removed=%d, want 1/1", len(kept), len(removed))
	}

	// Provenance is regenerated by the writer, so compare it separately.
	comparable := func(c model.ParsedContact) model.ParsedContact {
		c.Source, c.Raw, c.Malformed = "", "", false
		extra := make(map[string][]string, len(c.Extra))
		for k, v := range c.Extra {
			if k != "X-ROLODEX-SOURCE" {
				extra[k] = v
			}
		}
		c.Extra = extra
		return c
	}
	for i, pair := range [][2]model.ParsedContact{{input[0], kept[0]}, {input[1], removed[0]}} {
		want, got := comparable(pair[0]), comparable(pair[1])
		if !reflect.DeepEqual(want, got) {
			t.Errorf("contact %d differs after prune:\n want %+v\n got  %+v", i, want, got)
		}
	}
	if got := kept[0].Extra["X-ROLODEX-SOURCE"]; len(got) != 1 || got[0] != "icloud" {
		t.Errorf("provenance = %v, want [icloud]", got)
	}
	if got := removed[0].Extra["X-ROLODEX-SOURCE"]; len(got) != 0 {
		t.Errorf("foreign card gained provenance %v", got)
	}
}

// A run that fails part-way leaves every destination as it was. Before, the
// first file was written in place and then deleted on the way out, so a
// removed.vcf from an earlier run was replaced and unlinked by a run whose
// kept.vcf could not be written.
func TestPruneFailedRunLeavesDestinationsUntouched(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", pruneFixture)
	stale := writeFixture(t, dir, "removed.vcf", "PRECIOUS\r\n")

	// kept.vcf cannot be written: the pre-existing --removed file survives byte for byte.
	err := runPrune([]string{in, "--out", filepath.Join(dir, "no-such-dir", "kept.vcf"), "--removed", stale}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected an error writing kept.vcf into a missing directory")
	}
	if got := readFile(t, stale); got != "PRECIOUS\r\n" {
		t.Errorf("pre-existing --removed file was replaced or deleted: %q", got)
	}

	// removed.vcf cannot be written: no kept.vcf appears.
	kept := filepath.Join(dir, "kept.vcf")
	err = runPrune([]string{in, "--out", kept, "--removed", filepath.Join(dir, "no-such-dir", "removed.vcf")}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected an error writing removed.vcf into a missing directory")
	}
	if _, statErr := os.Stat(kept); statErr == nil {
		t.Error("kept.vcf written although removed.vcf could not be")
	}
	for _, name := range dirEntries(t, dir) {
		if strings.HasSuffix(name, ".tmp") {
			t.Errorf("staging file left behind: %s", name)
		}
	}
}

// An unknown flag is an error; -help is not one (the sibling commands'
// ExitOnError flag sets exit 0 after printing usage).
func TestPruneRejectsUnknownFlagAndHelpIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", pruneFixture)
	var err error
	captureStderr(t, func() { err = runPrune([]string{in, "--bogus"}, &bytes.Buffer{}) })
	if err == nil {
		t.Error("unknown flag accepted")
	}
	stderr := captureStderr(t, func() { err = runPrune([]string{"-help"}, &bytes.Buffer{}) })
	if err != nil {
		t.Errorf("-help returned an error: %v", err)
	}
	if !strings.Contains(stderr, "-reachable-by") {
		t.Errorf("-help did not print usage:\n%s", stderr)
	}
	if got := dirEntries(t, dir); len(got) != 1 {
		t.Errorf("files written: %v", got)
	}
}

// A contact with nothing but a name is counted and listed as "name only",
// and a single enabled channel is named without a conjunction.
func TestPruneNameOnlyAndSingleChannelRendering(t *testing.T) {
	dir := t.TempDir()
	bare := "BEGIN:VCARD\r\nVERSION:3.0\r\nN:Bare;Bo;;;\r\nFN:Bo Bare\r\nEND:VCARD\r\n"
	in := writeFixture(t, dir, "in.vcf", pruneFixture+bare)
	var stdout bytes.Buffer
	if err := runPrune([]string{in, "--out", filepath.Join(dir, "kept.vcf"), "--reachable-by", "url"}, &stdout); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Reachable by url: 0", "Unreachable: 5", "  name only: 1", "Bo Bare — has: name only", "Wrote 0 contacts -> "} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout.String())
		}
	}
}

// dispatch routes the command names it knows; the handlers are tested on
// their own, and "audit"/unknown are covered in TestAuditCommandRemoved.
func TestDispatchRoutes(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", pruneFixture)
	if err := dispatch("prune", []string{in, "--out", filepath.Join(dir, "kept.vcf"), "--format", "json"}); err != nil {
		t.Errorf("prune via dispatch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "removed.vcf")); err != nil {
		t.Errorf("prune via dispatch did not run: %v", err)
	}
	if err := dispatch("version", nil); err != nil {
		t.Errorf("version via dispatch: %v", err)
	}
}

// UID is the identity a CardDAV account matches on re-import, so it must
// survive the split byte for byte; before, the parser listed it as modeled
// and never stored it, and every re-imported card became a duplicate.
// PRODID and REV describe the file that was read, not the one written.
func TestPruneRoundTripKeepsUID(t *testing.T) {
	dir := t.TempDir()
	fixture := "BEGIN:VCARD\r\nVERSION:3.0\r\nPRODID:-//Apple Inc.//iOS 17//EN\r\nFN:Bob\r\nUID:urn:uuid:1234-abcd\r\nREV:2024-01-01T00:00:00Z\r\nEMAIL:bob@example.com\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Dee\r\nUID:dee-0001\r\nORG:Acme\r\nEND:VCARD\r\n"
	in := writeFixture(t, dir, "in.vcf", fixture)
	_, _, keptPath, removedPath := pruneOutputs(t, in)
	kept, removed := readFile(t, keptPath), readFile(t, removedPath)
	if strings.Count(kept, "UID:urn:uuid:1234-abcd\r\n") != 1 {
		t.Errorf("kept.vcf lost or doubled the UID:\n%s", kept)
	}
	if strings.Count(removed, "UID:dee-0001\r\n") != 1 {
		t.Errorf("removed.vcf lost or doubled the UID:\n%s", removed)
	}
	for _, dropped := range []string{"PRODID", "REV:"} {
		if strings.Contains(kept, dropped) {
			t.Errorf("%s describes the input file and should not be carried over:\n%s", dropped, kept)
		}
	}
}

// A foreign card cannot assert provenance rolodex never recorded: a value
// other than icloud, google or merged(icloud+google) is not stamped again.
func TestPruneIgnoresSpoofedProvenance(t *testing.T) {
	dir := t.TempDir()
	fixture := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Spoof\r\nEMAIL:s@example.com\r\nX-ROLODEX-SOURCE:merged(icloud+evil)\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Other\r\nEMAIL:o@example.com\r\nX-ROLODEX-SOURCE:outlook\r\nEND:VCARD\r\n"
	in := writeFixture(t, dir, "in.vcf", fixture)
	_, _, keptPath, _ := pruneOutputs(t, in)
	if kept := readFile(t, keptPath); strings.Contains(kept, "X-ROLODEX-SOURCE") {
		t.Errorf("spoofed provenance was re-stamped:\n%s", kept)
	}
}

// A card with no END:VCARD is a malformed entry like any other: the dry run
// reports it, --out refuses, --skip-malformed writes what could be read.
func TestPruneTreatsMissingENDAsMalformed(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Alice One\r\nEMAIL:alice@example.com\r\nEND:VCARD\r\n"+
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Bob Two\r\nEMAIL:bob@example.com\r\n"+
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Carol Three\r\nEMAIL:carol@example.com\r\nEND:VCARD\r\n")
	kept := filepath.Join(dir, "kept.vcf")
	var err error
	stderr := captureStderr(t, func() { err = runPrune([]string{in, "--out", kept}, &bytes.Buffer{}) })
	if err == nil || !strings.Contains(err.Error(), "1 malformed entries would be in neither output") {
		t.Errorf("error = %v, want the malformed refusal", err)
	}
	if !strings.Contains(stderr, "missing END:VCARD") {
		t.Errorf("stderr does not name the missing END:\n%s", stderr)
	}
	if got := dirEntries(t, dir); len(got) != 1 {
		t.Errorf("files written despite refusing: %v", got)
	}
	captureStderr(t, func() { err = runPrune([]string{in, "--out", kept, "--skip-malformed"}, &bytes.Buffer{}) })
	if err != nil {
		t.Fatal(err)
	}
	if k := readFile(t, kept); countCards(t, kept) != 1 || !strings.Contains(k, "FN:Alice One") || strings.Contains(k, "carol@") {
		t.Errorf("kept.vcf should hold Alice alone:\n%s", k)
	}
}

// A file with no vCard entries (a CSV renamed .vcf) is refused instead of
// being split into two empty files; an empty file is legitimately empty.
func TestPruneRefusesNonVCardInput(t *testing.T) {
	dir := t.TempDir()
	csv := writeFixture(t, dir, "contacts.vcf", "name,email\nAnn,ann@example.com\n")
	kept := filepath.Join(dir, "kept.vcf")
	stale := writeFixture(t, dir, "removed.vcf", pruneFixture)
	for _, args := range [][]string{{csv}, {csv, "--out", kept}, {csv, "--format", "json"}} {
		err := runPrune(args, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "contains no vCard entries") {
			t.Errorf("%v: error = %v, want a refusal", args, err)
		}
	}
	if _, err := os.Stat(kept); err == nil {
		t.Error("kept.vcf written for a file with no vCard entries")
	}
	if readFile(t, stale) != pruneFixture {
		t.Error("a pre-existing removed.vcf was replaced by a refused run")
	}

	empty := writeFixture(t, dir, "empty.vcf", "")
	var stdout bytes.Buffer
	if err := runPrune([]string{empty}, &stdout); err != nil {
		t.Errorf("an empty file is a legitimate empty address book: %v", err)
	}
	if !strings.Contains(stdout.String(), "Total contacts: 0") {
		t.Errorf("empty file report:\n%s", stdout.String())
	}
	// ...but there is nothing in it to split: --out is refused rather than
	// replacing two files with empty ones.
	err := runPrune([]string{empty, "--out", kept}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "is empty; nothing to split") {
		t.Errorf("error = %v, want a refusal", err)
	}
	if readFile(t, stale) != pruneFixture {
		t.Error("a pre-existing removed.vcf was replaced by a run on an empty file")
	}
}

// A PHOTO given as a URI is written back as one. Before, the URI's bytes
// were base64-encoded under ENCODING=b, and every Google photo became an
// unreadable blob.
func TestPruneRoundTripsPhotoURI(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:A\r\nEMAIL:a@example.com\r\nPHOTO;VALUE=uri:https://example.com/a.jpg\r\nEND:VCARD\r\n")
	_, _, keptPath, _ := pruneOutputs(t, in)
	kept := readFile(t, keptPath)
	if !strings.Contains(kept, "PHOTO;VALUE=uri:https://example.com/a.jpg\r\n") || strings.Contains(kept, "ENCODING=b") {
		t.Errorf("photo URI not written back as a URI:\n%s", kept)
	}

	// A URI is not escaped text: a data: URI's ';' and ',' and a query's
	// ',' come back byte for byte, pass after pass.
	uri := "data:image/png;base64,iVBORw0KGgo=,x"
	in = writeFixture(t, dir, "data.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:B\r\nEMAIL:b@example.com\r\nPHOTO;VALUE=uri:"+uri+"\r\nEND:VCARD\r\n")
	for pass := 0; pass < 2; pass++ {
		_, _, keptPath, _ = pruneOutputs(t, in)
		if kept := readFile(t, keptPath); !strings.Contains(kept, "PHOTO;VALUE=uri:"+uri+"\r\n") {
			t.Fatalf("pass %d: data URI not byte-identical:\n%s", pass, kept)
		}
		// Feed the output back in from its own directory (outputs land
		// beside the input).
		sub := filepath.Join(dir, "pass"+string(rune('1'+pass)))
		if err := os.Mkdir(sub, 0o700); err != nil {
			t.Fatal(err)
		}
		in = writeFixture(t, sub, "in.vcf", readFile(t, keptPath))
	}
}

// The report says what a removed contact has, including a phone or email
// that did not count and a note, so "name only" means exactly that.
func TestPruneReportNamesUncountedPhoneAndNote(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Short Code\r\nTEL:611\r\nNOTE:cable company support line\r\nEND:VCARD\r\n"+
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Just Name\r\nEND:VCARD\r\n")
	var stdout bytes.Buffer
	if err := runPrune([]string{in}, &stdout); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"  with an email or phone that did not count: 1",
		"  name only: 1",
		"Short Code — has: phone, note",
		"Just Name — has: name only",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout.String())
		}
	}
	var got map[string]any
	if err := runPrune([]string{in, "--format", "json"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(stdout.Bytes()[strings.Index(stdout.String(), "{"):], &got); err != nil {
		t.Fatal(err)
	}
	first, _ := got["removed_contacts"].([]any)[0].(map[string]any)
	if first["has_phone"] != true || first["has_note"] != true || first["has_email"] != false {
		t.Errorf("removed_contacts[0] = %v", first)
	}
}

// With --format json the refusal over malformed entries is stderr's to
// explain: the JSON report is never printed, so the warning lines are the
// only record of what would be lost.
func TestPruneMalformedJSONRefusalStillWarns(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "in.vcf", malformedFixture)
	kept := filepath.Join(dir, "kept.vcf")

	var stdout bytes.Buffer
	var err error
	stderr := captureStderr(t, func() {
		err = runPrune([]string{in, "--out", kept, "--format", "json"}, &stdout)
	})
	if err == nil || !strings.Contains(err.Error(), "malformed entries would be in neither output") {
		t.Errorf("err = %v, want the refusal", err)
	}
	if !strings.Contains(stderr, "warning: 1 malformed entries in "+in) {
		t.Errorf("stderr lacks the warning detail:\n%s", stderr)
	}
	if stdout.Len() != 0 {
		t.Errorf("refusal should print no JSON report, got:\n%s", stdout.String())
	}
	if got := dirEntries(t, dir); len(got) != 1 {
		t.Errorf("files written despite refusing: %v", got)
	}
}

// sameFile answers false, never errors, when either side cannot be stat'd —
// a missing file must not block the caller's normal path.
func TestSameFileMissingPaths(t *testing.T) {
	dir := t.TempDir()
	real := writeFixture(t, dir, "real.vcf", pruneFixture)
	missing := filepath.Join(dir, "missing.vcf")
	if sameFile(missing, real) {
		t.Error("missing first path compared as the same file")
	}
	if sameFile(real, missing) {
		t.Error("missing second path compared as the same file")
	}
	if !sameFile(real, real) {
		t.Error("a path is the same file as itself")
	}
}
