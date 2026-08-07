package main

import (
	"fmt"
	"os"

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

	fmt.Println("Parsing Google contacts...")
	googleContacts, googleWarnings, err := parser.ParseFile(googlePath, model.SourceGoogle)
	if err != nil {
		return nil, fmt.Errorf("parsing Google file: %w", err)
	}
	fmt.Printf("  %d contacts loaded\n", len(googleContacts))

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

	return &PipelineResult{
		MergeResult: result,
		Normalized:  normalized,
		Warnings:    allWarnings,
		ICloudCount: len(icloudContacts),
		GoogleCount: len(googleContacts),
		AutoCount:   autoCount,
	}, nil
}

func merge(icloudPath, googlePath, outPath, reviewPath, reportPath string) error {
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

	// Write review.vcf (always write to avoid stale files from prior runs)
	if len(result.Review) > 0 {
		fmt.Printf("Writing %d review contacts → %s\n", len(result.Review), reviewPath)
		if err := writer.WriteFile(reviewPath, result.Review); err != nil {
			return fmt.Errorf("writing review output: %w", err)
		}
	} else {
		// Remove any stale review.vcf from a previous run
		os.Remove(reviewPath)
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
