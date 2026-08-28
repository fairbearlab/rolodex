package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fairbearlab/rolodex/internal/blocker"
	"github.com/fairbearlab/rolodex/internal/merger"
	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
	"github.com/fairbearlab/rolodex/internal/parser"
	"github.com/fairbearlab/rolodex/internal/reporter"
	resolveCmd "github.com/fairbearlab/rolodex/internal/resolve"
	reviewCmd "github.com/fairbearlab/rolodex/internal/review"
	"github.com/fairbearlab/rolodex/internal/scorer"
	"github.com/fairbearlab/rolodex/internal/writer"
)

// PipelineResult holds the output of the merge pipeline before any file I/O.
type PipelineResult struct {
	MergeResult merger.Result
	Normalized  []model.NormalizedContact
	Warnings    []model.Warning
	ICloudCount int
	GoogleCount int
	AutoCount   int
}

// reportParseWarnings tells the user, on stderr, about entries the decoder
// could not read. A malformed card is skipped and its contact is gone from
// every output, but the "N contacts loaded" line is counted AFTER the loss, so
// nothing on screen revealed it — a truncated export (an interrupted download,
// a full disk) silently shrank the address book and the command exited 0.
// audit already surfaces these; the merge pipeline did not.
func reportParseWarnings(path string, warnings []model.Warning) {
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "  warning: %d malformed entries in %s were skipped and are NOT in the output:\n",
		len(warnings), path)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "    - entry %d: %s\n", w.Index, w.Message)
	}
}

// runPipeline executes the core merge pipeline (parse → normalize → block →
// score → merge) and returns the in-memory result without writing any files.
func runPipeline(icloudPath, googlePath string) (*PipelineResult, error) {
	// Stage 1: Parse
	fmt.Println("Parsing iCloud contacts...")
	icloudContacts, icloudWarnings, err := parser.ParseFile(icloudPath, model.SourceICloud)
	if err != nil {
		return nil, fmt.Errorf("parsing iCloud file: %w", err)
	}
	fmt.Printf("  %d contacts loaded\n", len(icloudContacts))
	reportParseWarnings(icloudPath, icloudWarnings)

	fmt.Println("Parsing Google contacts...")
	googleContacts, googleWarnings, err := parser.ParseFile(googlePath, model.SourceGoogle)
	if err != nil {
		return nil, fmt.Errorf("parsing Google file: %w", err)
	}
	fmt.Printf("  %d contacts loaded\n", len(googleContacts))
	reportParseWarnings(googlePath, googleWarnings)

	// Combine all contacts and warnings
	allContacts := append(icloudContacts, googleContacts...)
	allWarnings := append(icloudWarnings, googleWarnings...)

	// Stage 2: Normalize
	fmt.Println("Normalizing...")
	normalized := make([]model.NormalizedContact, len(allContacts))
	for i, c := range allContacts {
		normalized[i] = normalize.Contact(c)
	}

	// Stage 3: Block
	fmt.Println("Blocking candidates...")
	pairs := blocker.Block(normalized)
	fmt.Printf("  %d candidate pairs\n", len(pairs))

	// Stage 4: Score
	fmt.Println("Scoring pairs...")
	scored := scorer.Score(normalized, pairs)
	autoCount, reviewCount := 0, 0
	for _, s := range scored {
		switch s.Tier {
		case model.TierAutoMerge:
			autoCount++
		case model.TierReview:
			reviewCount++
		}
	}
	fmt.Printf("  %d auto-merge, %d review, %d distinct\n",
		autoCount, reviewCount, len(scored)-autoCount-reviewCount)

	// Stage 5: Merge
	fmt.Println("Merging...")
	result := merger.Merge(normalized, scored)
	if n := len(result.Deferred); n > 0 {
		// The "N review" count above is pairs; these pairs will not get a
		// card, so say so here rather than let the report's smaller review
		// count look like a discrepancy.
		fmt.Printf("  %d same-name pair(s) not reviewed: one side is already merged on a shared identifier "+
			"(kept as separate people; listed under \"deferred\" in the report)\n", n)
	}

	return &PipelineResult{
		MergeResult: result,
		Normalized:  normalized,
		Warnings:    allWarnings,
		ICloudCount: len(icloudContacts),
		GoogleCount: len(googleContacts),
		AutoCount:   autoCount,
	}, nil
}

// sameFile reports whether two paths resolve to the same file on disk. It
// answers false when either cannot be stat'd, so a missing file never blocks
// the caller's normal path.
func sameFile(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(fa, fb)
}

// isRolodexReviewFile reports whether path holds a review.vcf written by
// rolodex. Every card the merger routes to review carries X-ROLODEX-REVIEW,
// so its presence separates our own stale artifact from a file that merely
// shares the default name. An unreadable or empty file is not ours.
func isRolodexReviewFile(path string) bool {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return false
	}
	return bytes.Contains(data, []byte("X-ROLODEX-REVIEW:true"))
}

func merge(icloudPath, googlePath, outPath, reviewPath, reportPath string, reviewDerived bool) error {
	pr, err := runPipeline(icloudPath, googlePath)
	if err != nil {
		return err
	}
	result := pr.MergeResult

	// Write merged.vcf
	fmt.Printf("Writing %d merged contacts → %s\n", len(result.Merged), outPath)
	if err := writer.WriteFile(outPath, result.Merged); err != nil {
		return fmt.Errorf("writing merged output: %w", err)
	}

	// Write review.vcf, or remove a stale one from a previous run: report.json
	// would say "review": [] while review.vcf still held the old contacts, and
	// resolve then refused the pair as misaligned.
	if len(result.Review) > 0 {
		fmt.Printf("Writing %d review contacts → %s\n", len(result.Review), reviewPath)
		if err := writer.WriteFile(reviewPath, result.Review); err != nil {
			return fmt.Errorf("writing review output: %w", err)
		}
	} else if !sameFile(reviewPath, outPath) && (!reviewDerived || isRolodexReviewFile(reviewPath)) {
		// A path the user named is theirs to clear. --review defaults to a
		// path derived from --out, and deleting whatever sat there meant
		// "merge --out ~/Documents/merged.vcf" removed ~/Documents/review.vcf
		// — a file this run never created and the user never mentioned. So a
		// derived path is only cleared when the file is one rolodex wrote.
		// The sameFile check is a second line of defence behind
		// checkDistinctPaths: on a case-insensitive filesystem "--out
		// Merged.vcf --review merged.vcf" are one file, and removing it here
		// deleted the merged output that had just been written.
		_ = os.Remove(reviewPath)
	}

	// Report
	if reportPath != "" {
		report := reporter.Generate(pr.Normalized, result,
			pr.ICloudCount, pr.GoogleCount, pr.Warnings)
		if err := reporter.WriteFile(reportPath, report); err != nil {
			return fmt.Errorf("writing report: %w", err)
		}
		fmt.Printf("Report → %s\n", reportPath)
	}

	fmt.Println("Done.")
	return nil
}

func reviewInteractive(reportPath, reviewPath, calibrationPath string) error {
	_, err := reviewCmd.Run(reportPath, reviewPath, calibrationPath)
	return err
}

func resolve(reportPath, reviewPath, mergedPath, outPath string) error {
	return resolveCmd.Run(reportPath, reviewPath, mergedPath, outPath)
}
