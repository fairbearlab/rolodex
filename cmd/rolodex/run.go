package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/fairbearlab/rolodex/internal/reporter"
	resolveCmd "github.com/fairbearlab/rolodex/internal/resolve"
	reviewCmd "github.com/fairbearlab/rolodex/internal/review"
	"github.com/fairbearlab/rolodex/internal/writer"
)

// ErrReviewPaused is returned when the user exits the review TUI with
// pending decisions. Callers should treat this as an incomplete run.
var ErrReviewPaused = errors.New("review paused with pending decisions")

func run(icloudPath, googlePath, outPath, reportSavePath string, keep bool) error {
	// Reject overlapping output paths to avoid silent overwrites.
	absOut, _ := filepath.Abs(outPath)
	absCalibration, _ := filepath.Abs(filepath.Join(filepath.Dir(outPath), "calibration.jsonl"))

	// When --keep is enabled, merged.vcf and review.vcf in the output
	// directory are also reserved (they get copied from the temp workspace).
	absMergedKeep, _ := filepath.Abs(filepath.Join(filepath.Dir(outPath), "merged.vcf"))
	absReviewKeep, _ := filepath.Abs(filepath.Join(filepath.Dir(outPath), "review.vcf"))

	if reportSavePath != "" {
		absReport, _ := filepath.Abs(reportSavePath)
		if absOut == absReport {
			return fmt.Errorf("--report and --out cannot point to the same file (%s)", outPath)
		}
		if absReport == absCalibration {
			return fmt.Errorf("--report cannot point to %s (reserved for calibration data)", reportSavePath)
		}
		if keep {
			if absReport == absMergedKeep {
				return fmt.Errorf("--report cannot point to %s (reserved by --keep for merged contacts)", reportSavePath)
			}
			if absReport == absReviewKeep {
				return fmt.Errorf("--report cannot point to %s (reserved by --keep for review contacts)", reportSavePath)
			}
		}
	}
	if absOut == absCalibration {
		return fmt.Errorf("--out cannot point to %s (reserved for calibration data)", outPath)
	}

	// Validate output paths before running the pipeline
	if err := os.MkdirAll(filepath.Dir(outPath), 0750); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	if reportSavePath != "" {
		if err := os.MkdirAll(filepath.Dir(reportSavePath), 0750); err != nil {
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
	paused := false
	defer func() {
		if succeeded {
			_ = os.RemoveAll(tempDir)
		} else if !paused {
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

		reviewComplete, err := reviewCmd.Run(tempReportPath, tempReviewPath, tempCalibrationPath)
		if err != nil {
			return fmt.Errorf("review: %w", err)
		}

		if !reviewComplete {
			paused = true
			fmt.Fprintf(os.Stderr, "\nReview paused with pending decisions.\n")
			fmt.Fprintf(os.Stderr, "Workspace preserved: %s\n", tempDir)
			fmt.Fprintf(os.Stderr, "Resume with: rolodex review --report %s --review %s\n", tempReportPath, tempReviewPath)
			fmt.Fprintf(os.Stderr, "Then resolve: rolodex resolve --report %s --review %s --merged %s --out %s\n",
				tempReportPath, tempReviewPath, tempMergedPath, outPath)
			return ErrReviewPaused
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
		if err := copyFile(tempReportPath, reportSavePath); err != nil {
			return fmt.Errorf("saving report: %w", err)
		}
		fmt.Printf("Report → %s\n", reportSavePath)
	}

	// Append calibration data alongside output if it was generated.
	// Use O_APPEND to preserve entries from prior sessions (calibration
	// is an accumulating log, not a per-run snapshot).
	calDst := filepath.Join(filepath.Dir(outPath), "calibration.jsonl")
	wroteCalibration := false
	if hasReview {
		if calData, err := os.ReadFile(filepath.Clean(tempCalibrationPath)); err == nil && len(calData) > 0 {
			f, err := os.OpenFile(filepath.Clean(calDst), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
			if err != nil {
				return fmt.Errorf("saving calibration: %w", err)
			}
			_, writeErr := f.Write(calData)
			closeErr := f.Close()
			if writeErr != nil {
				return fmt.Errorf("saving calibration: %w", writeErr)
			}
			if closeErr != nil {
				return fmt.Errorf("saving calibration: %w", closeErr)
			}
			wroteCalibration = true
			fmt.Printf("Calibration → %s\n", calDst)
		}
	}
	// When --keep is set and no calibration was written this run,
	// remove any stale calibration.jsonl from a previous --keep run
	// so callers don't read old data.
	if keep && !wroteCalibration {
		_ = os.Remove(calDst)
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
			// calibration.jsonl is NOT included here because it is
			// already handled above via O_APPEND (accumulating log).
			// A full os.WriteFile here would overwrite the accumulated
			// multi-session data with just this session's entries.
		}
		absOutPath, _ := filepath.Abs(outPath)
		for _, f := range filesToKeep {
			dst := filepath.Join(outDir, f.name)
			// Skip if this would overwrite the final resolved output
			if absDst, _ := filepath.Abs(dst); absDst == absOutPath {
				continue
			}
			if err := copyFile(f.src, dst); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					// Source doesn't exist this run (e.g., no review.vcf
					// when no review pairs). Remove any stale copy from a
					// previous --keep run so callers don't read old data.
					_ = os.Remove(dst)
					continue
				}
				return fmt.Errorf("keeping %s: %w", f.name, err)
			}
			fmt.Printf("Kept %s\n", dst)
		}
	}

	succeeded = true
	fmt.Println("Done.")
	return nil
}

// copyFile streams src to dst (mode 0600, truncating any existing file).
// Errors from opening src are returned unwrapped so callers can test them
// with errors.Is(err, os.ErrNotExist).
func copyFile(src, dst string) error {
	in, err := os.Open(filepath.Clean(src))
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(filepath.Clean(dst), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("writing %s: %w", dst, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", dst, err)
	}
	return nil
}
