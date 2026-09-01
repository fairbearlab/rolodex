package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/parser"
	"github.com/fairbearlab/rolodex/internal/prune"
	"github.com/fairbearlab/rolodex/internal/writer"
)

// pruneOptions is a parsed "rolodex prune" invocation.
type pruneOptions struct {
	input         string
	out           string // "" is a dry run
	removed       string
	reachableBy   []prune.Channel
	format        string
	skipMalformed bool
}

// runPrune parses the prune command line and runs it, printing to stdout.
func runPrune(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("out", "", "write reachable contacts here (default: dry run, write nothing)")
	removed := fs.String("removed", "", "write unreachable contacts here (default: removed.vcf next to --out)")
	reachableBy := fs.String("reachable-by", "email,phone,address", "channels that make a contact reachable: email, phone, address, url")
	format := fs.String("format", "text", "output format: text or json")
	skipMalformed := fs.Bool("skip-malformed", false, "with --out, write the files even when malformed entries would be lost")

	// flag.FlagSet stops at the first non-flag token, so "prune file.vcf
	// --out kept.vcf" would silently ignore --out and run a dry run.
	if err := fs.Parse(reorderFlagsBeforePositional(args, fs)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			// The usage was printed; "-help" is not an error, as the
			// ExitOnError flag sets of the sibling commands treat it.
			return nil
		}
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: rolodex prune <file.vcf> [--out kept.vcf] [--removed removed.vcf] [--reachable-by email,phone,address] [--format text|json] [--skip-malformed]")
	}
	if *format != "text" && *format != "json" {
		return fmt.Errorf("unknown format %q (use text or json)", *format)
	}
	channels, err := prune.ParseChannels(*reachableBy)
	if err != nil {
		return err
	}
	opts := pruneOptions{
		input:         fs.Arg(0),
		out:           *out,
		removed:       *removed,
		reachableBy:   channels,
		format:        *format,
		skipMalformed: *skipMalformed,
	}
	if opts.out == "" && opts.removed != "" {
		return fmt.Errorf("--removed requires --out; without --out prune is a dry run and writes nothing")
	}
	if opts.out != "" {
		if opts.removed == "" {
			opts.removed = filepath.Join(filepath.Dir(opts.out), "removed.vcf")
		}
		inputs := []pathFlag{{"<file>", opts.input}}
		outputs := []pathFlag{{"--out", opts.out}, {"--removed", opts.removed}}
		if err := checkDistinctPaths(inputs, outputs); err != nil {
			return err
		}
	}
	return runPruneCmd(opts, stdout)
}

func runPruneCmd(opts pruneOptions, stdout io.Writer) error {
	contacts, warnings, err := parser.ParseFile(opts.input, model.SourceUnknown)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", opts.input, err)
	}
	if opts.format == "text" {
		reportParseWarnings(opts.input, warnings)
	}
	dryRun := opts.out == ""
	if len(contacts) == 0 && len(warnings) == 0 {
		// go-vcard skips every line it cannot parse, so a CSV named .vcf
		// decodes to zero contacts and zero warnings; --out would then
		// write two empty files over whatever was there and exit 0. An
		// empty file is a legitimate empty address book to report on, but
		// there is nothing in it to split either.
		if fi, statErr := os.Stat(opts.input); statErr == nil && fi.Size() > 0 {
			return fmt.Errorf("%s contains no vCard entries; is it a .vcf file?", opts.input)
		}
		if !dryRun {
			return fmt.Errorf("%s is empty; nothing to split", opts.input)
		}
	}

	result := prune.Split(contacts, prune.Options{ReachableBy: opts.reachableBy})

	if !dryRun {
		// A malformed card is in neither output: after the run the user
		// holds two files that together are less than the input, and nothing
		// on disk says so. The dry run reports; the write refuses.
		if len(warnings) > 0 && !opts.skipMalformed {
			if opts.format == "json" {
				// The refusal is the only output; a warning about a card
				// that absorbed several others is where the real count is.
				reportParseWarnings(opts.input, warnings)
			}
			return fmt.Errorf("%d malformed entries would be in neither output; fix the file or pass --skip-malformed", len(warnings))
		}
		if err := writePruned(opts, result); err != nil {
			return err
		}
	}

	if opts.format == "json" {
		return printPruneJSON(stdout, opts, result, warnings, dryRun)
	}
	_, err = io.WriteString(stdout, pruneText(opts, result, dryRun))
	return err
}

// writePruned stages both files before either is moved into place, so a
// failed run leaves every destination exactly as it was: a removed.vcf from
// an earlier run is not replaced and then deleted on the way out, and no
// run leaves exactly one of the two files. removed.vcf is written even when
// empty: a stale one from an earlier run must not survive a successful run,
// and an empty file is unambiguous.
func writePruned(opts pruneOptions, result prune.Result) error {
	removed, err := writer.Stage(opts.removed, asMerged(result.Removed))
	if err != nil {
		return fmt.Errorf("writing %s: %w", opts.removed, err)
	}
	kept, err := writer.Stage(opts.out, asMerged(result.Kept))
	if err != nil {
		removed.Abort()
		return fmt.Errorf("writing %s: %w", opts.out, err)
	}
	if err := removed.Commit(); err != nil {
		kept.Abort()
		return fmt.Errorf("writing %s: %w", opts.removed, err)
	}
	if err := kept.Commit(); err != nil {
		// Both files were fully written and the rename of a sibling in the
		// same directory just succeeded; a failure here is an I/O fault the
		// user needs to hear about, including that removed.vcf did land.
		return fmt.Errorf("writing %s (%s was already replaced): %w", opts.out, opts.removed, err)
	}
	return nil
}

// asMerged wraps parsed contacts for the writer, restoring the provenance
// an earlier rolodex run recorded so it is stamped once, unchanged. A
// foreign card has none and gets none.
func asMerged(contacts []model.ParsedContact) []model.MergedContact {
	out := make([]model.MergedContact, len(contacts))
	for i, c := range contacts {
		out[i] = model.MergedContact{Contact: c, Sources: parser.Provenance(c)}
	}
	return out
}

func printPruneJSON(w io.Writer, opts pruneOptions, result prune.Result, warnings []model.Warning, dryRun bool) error {
	type jsonOutput struct {
		Total           int             `json:"total"`
		Kept            int             `json:"kept"`
		Removed         int             `json:"removed"`
		ReachableBy     []prune.Channel `json:"reachable_by"`
		DryRun          bool            `json:"dry_run"`
		Out             string          `json:"out"`
		RemovedPath     string          `json:"removed_path"`
		RemovedContacts []prune.Removed `json:"removed_contacts"`
		WarningCount    int             `json:"warning_count"`
		Warnings        []model.Warning `json:"warnings"`
	}
	out := jsonOutput{
		Total:           result.Total,
		Kept:            len(result.Kept),
		Removed:         len(result.Removed),
		ReachableBy:     opts.reachableBy,
		DryRun:          dryRun,
		Out:             opts.out,
		RemovedPath:     opts.removed,
		RemovedContacts: result.Detail,
		WarningCount:    len(warnings),
		Warnings:        warnings,
	}
	if out.RemovedContacts == nil {
		out.RemovedContacts = []prune.Removed{}
	}
	if out.Warnings == nil {
		out.Warnings = []model.Warning{}
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

// pruneText renders the report; after a write the same block precedes the
// two "Wrote" lines.
func pruneText(opts pruneOptions, result prune.Result, dryRun bool) string {
	w := &strings.Builder{}
	title := "Contact prune"
	if dryRun {
		title += " (dry run)"
	}
	fmt.Fprintln(w, title)
	fmt.Fprintln(w, strings.Repeat("━", 39))
	fmt.Fprintf(w, "Total contacts: %d\n", result.Total)
	fmt.Fprintf(w, "Reachable by %s: %d\n", describeChannels(opts.reachableBy), len(result.Kept))
	fmt.Fprintf(w, "Unreachable: %d\n", len(result.Removed))

	var uncounted, orgOrTitle, url, birthday, nameOnly int
	for _, d := range result.Detail {
		if d.HasEmail || d.HasPhone {
			uncounted++
		}
		if d.HasOrg || d.HasTitle {
			orgOrTitle++
		}
		if d.HasURL {
			url++
		}
		if d.HasBirthday {
			birthday++
		}
		if !d.HasEmail && !d.HasPhone && !d.HasOrg && !d.HasTitle && !d.HasAddress &&
			!d.HasURL && !d.HasBirthday && !d.HasNote && !d.HasPhoto {
			nameOnly++
		}
	}
	fmt.Fprintf(w, "  with an email or phone that did not count: %d\n", uncounted)
	fmt.Fprintf(w, "  with org or title: %d\n", orgOrTitle)
	fmt.Fprintf(w, "  with URL: %d\n", url)
	fmt.Fprintf(w, "  with birthday: %d\n", birthday)
	fmt.Fprintf(w, "  name only: %d\n", nameOnly)

	if len(result.Detail) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Unreachable contacts:")
		for i, d := range result.Detail {
			fmt.Fprintf(w, "  %d. %s — has: %s\n", i+1, d.Name, describeHas(d))
		}
	}

	if dryRun {
		if result.Total == 0 {
			fmt.Fprintln(w, "No contacts; nothing to split.")
			return w.String()
		}
		fmt.Fprintf(w, "%d contacts would be removed. Re-run with --out kept.vcf to write kept.vcf and removed.vcf.\n", len(result.Removed))
		return w.String()
	}
	fmt.Fprintf(w, "Wrote %d contacts -> %s\n", len(result.Kept), opts.out)
	fmt.Fprintf(w, "Wrote %d contacts -> %s\n", len(result.Removed), opts.removed)
	return w.String()
}

// describeChannels renders "email, phone, or address".
func describeChannels(chs []prune.Channel) string {
	names := make([]string, len(chs))
	for i, ch := range chs {
		names[i] = string(ch)
	}
	switch len(names) {
	case 1:
		return names[0]
	case 2:
		return names[0] + " or " + names[1]
	default:
		return strings.Join(names[:len(names)-1], ", ") + ", or " + names[len(names)-1]
	}
}

// describeHas lists what a removed contact carries. An email or phone here
// is one that did not count (a placeholder, or a channel that is off).
func describeHas(d prune.Removed) string {
	var parts []string
	if d.HasEmail {
		parts = append(parts, "email")
	}
	if d.HasPhone {
		parts = append(parts, "phone")
	}
	if d.HasOrg {
		parts = append(parts, "org")
	}
	if d.HasTitle {
		parts = append(parts, "title")
	}
	if d.HasAddress {
		parts = append(parts, "address")
	}
	if d.HasURL {
		parts = append(parts, "url")
	}
	if d.HasBirthday {
		parts = append(parts, "birthday")
	}
	if d.HasNote {
		parts = append(parts, "note")
	}
	if d.HasPhoto {
		parts = append(parts, "photo")
	}
	if len(parts) == 0 {
		return "name only"
	}
	return strings.Join(parts, ", ")
}
