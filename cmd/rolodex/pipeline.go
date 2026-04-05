package main

import (
	"fmt"

	"github.com/fairbearlabs/rolodex/internal/blocker"
	"github.com/fairbearlabs/rolodex/internal/merger"
	"github.com/fairbearlabs/rolodex/internal/model"
	"github.com/fairbearlabs/rolodex/internal/normalize"
	"github.com/fairbearlabs/rolodex/internal/parser"
	"github.com/fairbearlabs/rolodex/internal/reporter"
	resolveCmd "github.com/fairbearlabs/rolodex/internal/resolve"
	"github.com/fairbearlabs/rolodex/internal/scorer"
	"github.com/fairbearlabs/rolodex/internal/writer"
)

func merge(icloudPath, googlePath, outPath, reviewPath, reportPath string) error {
	// Stage 1: Parse
	fmt.Println("Parsing iCloud contacts...")
	icloudContacts, icloudWarnings, err := parser.ParseFile(icloudPath, model.SourceICloud)
	if err != nil {
		return fmt.Errorf("parsing iCloud file: %w", err)
	}
	fmt.Printf("  %d contacts loaded\n", len(icloudContacts))

	fmt.Println("Parsing Google contacts...")
	googleContacts, googleWarnings, err := parser.ParseFile(googlePath, model.SourceGoogle)
	if err != nil {
		return fmt.Errorf("parsing Google file: %w", err)
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

	// Stage 6: Write merged.vcf
	fmt.Printf("Writing %d merged contacts → %s\n", len(result.Merged), outPath)
	if err := writer.WriteFile(outPath, result.Merged); err != nil {
		return fmt.Errorf("writing merged output: %w", err)
	}

	// Stage 7: Write review.vcf
	if len(result.Review) > 0 {
		fmt.Printf("Writing %d review contacts → %s\n", len(result.Review), reviewPath)
		if err := writer.WriteFile(reviewPath, result.Review); err != nil {
			return fmt.Errorf("writing review output: %w", err)
		}
	}

	// Stage 8: Report
	if reportPath != "" {
		report := reporter.Generate(normalized, result,
			len(icloudContacts), len(googleContacts), allWarnings)
		if err := reporter.WriteFile(reportPath, report); err != nil {
			return fmt.Errorf("writing report: %w", err)
		}
		fmt.Printf("Report → %s\n", reportPath)
	}

	fmt.Println("Done.")
	return nil
}

func resolve(reportPath, reviewPath, mergedPath, outPath string) error {
	return resolveCmd.Run(reportPath, reviewPath, mergedPath, outPath)
}
