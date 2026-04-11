package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fairbearlab/rolodex/internal/reporter"
	reviewCmd "github.com/fairbearlab/rolodex/internal/review"
	resolveCmd "github.com/fairbearlab/rolodex/internal/resolve"
	"github.com/fairbearlab/rolodex/internal/writer"
)

func run(icloudPath, googlePath, outPath, reportSavePath string, keep bool) error {
	// Reject --report paths that overlap --out to avoid replacing the
	// resolved VCF with JSON.
	if reportSavePath != "" {
		absOut, _ := filepath.Abs(outPath)
		absReport, _ := filepath.Abs(reportSavePath)
		if absOut == absReport {
			return fmt.Errorf("--report and --out cannot point to the same file (%s)", outPath)
		}
	}

	// Validate output paths before running the pipeline
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	if reportSavePath != "" {
		if err := os.MkdirAll(filepath.Dir(reportSavePath), 0755); err != nil {
			return fmt.Errorf("creating report directory: %w", err)
		}
	}

	// Run the merge pipeline
	pr, err := runPipeline(icloudPath, googlePath)
	if err != nil {
		return err
	}
	result := pr.MergeResult

	// Create temp dir for intermediates
	tempDir, err := os.MkdirTemp("", "rolodex-run-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	// Clean up temp dir only on success; preserve on error so review
	// decisions in report.json and review.vcf can be recovered.
	succeeded := false
	defer func() {
		if succeeded {
			os.RemoveAll(tempDir)
		} else {
			fmt.Fprintf(os.Stderr, "\nTemp workspace preserved: %s\n", tempDir)
		}
	}()

	tempMergedPath := filepath.Join(tempDir, "merged.vcf")
	tempReviewPath := filepath.Join(tempDir, "review.vcf")
	tempReportPath := filepath.Join(tempDir, "report.json")
	tempCalibrationPath := filepath.Join(tempDir, "calibration.jsonl")

	// Write merged.vcf to temp dir
	if err := writer.WriteFile(tempMergedPath, result.Merged); err != nil {
		return fmt.Errorf("writing merged contacts: %w", err)
	}

	// Generate and write report to temp dir
	report := reporter.Generate(pr.Normalized, result,
		pr.ICloudCount, pr.GoogleCount, pr.Warnings)
	if err := reporter.WriteFile(tempReportPath, report); err != nil {
		return fmt.Errorf("writing report: %w", err)
	}

	// If there are review-tier pairs, write review.vcf and launch TUI
	hasReview := len(result.Review) > 0
	if hasReview {
		if err := writer.WriteFile(tempReviewPath, result.Review); err != nil {
			return fmt.Errorf("writing review contacts: %w", err)
		}

		fmt.Printf("%d contacts need review. Launching review...\n\n", len(result.Review))

		if err := reviewCmd.Run(tempReportPath, tempReviewPath, tempCalibrationPath); err != nil {
			return fmt.Errorf("review: %w", err)
		}

		fmt.Println()
	} else {
		fmt.Println("No uncertain matches — skipping review.")
	}

	// Resolve: combine merged + reviewed into final output
	fmt.Println("Resolving...")
	if hasReview {
		if err := resolveCmd.Run(tempReportPath, tempReviewPath, tempMergedPath, outPath); err != nil {
			return fmt.Errorf("resolve: %w", err)
		}
	} else {
		// No review contacts, just copy merged directly to output
		if err := writer.WriteFile(outPath, result.Merged); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		fmt.Printf("Resolved %d contacts → %s\n", len(result.Merged), outPath)
	}

	// Save report if requested
	if reportSavePath != "" {
		data, err := os.ReadFile(tempReportPath)
		if err != nil {
			return fmt.Errorf("reading report: %w", err)
		}
		if err := os.WriteFile(reportSavePath, data, 0600); err != nil {
			return fmt.Errorf("saving report: %w", err)
		}
		fmt.Printf("Report → %s\n", reportSavePath)
	}

	// Save calibration data alongside output if it was generated
	if hasReview {
		if calData, err := os.ReadFile(tempCalibrationPath); err == nil && len(calData) > 0 {
			calDst := filepath.Join(filepath.Dir(outPath), "calibration.jsonl")
			if err := os.WriteFile(calDst, calData, 0600); err != nil {
				return fmt.Errorf("saving calibration: %w", err)
			}
			fmt.Printf("Calibration → %s\n", calDst)
		}
	}

	// Copy intermediates if --keep
	if keep {
		outDir := filepath.Dir(outPath)
		filesToKeep := []struct {
			src  string
			name string
		}{
			{tempMergedPath, "merged.vcf"},
			{tempReviewPath, "review.vcf"},
			{tempCalibrationPath, "calibration.jsonl"},
		}
		absOutPath, _ := filepath.Abs(outPath)
		for _, f := range filesToKeep {
			data, err := os.ReadFile(f.src)
			if err != nil {
				if os.IsNotExist(err) {
					continue // file may not exist (e.g., no review.vcf when no review pairs)
				}
				return fmt.Errorf("reading %s for --keep: %w", f.name, err)
			}
			dst := filepath.Join(outDir, f.name)
			// Skip if this would overwrite the final resolved output
			if absDst, _ := filepath.Abs(dst); absDst == absOutPath {
				continue
			}
			if err := os.WriteFile(dst, data, 0600); err != nil {
				return fmt.Errorf("keeping %s: %w", f.name, err)
			}
			fmt.Printf("Kept %s\n", dst)
		}
	}

	succeeded = true
	fmt.Println("Done.")
	return nil
}
