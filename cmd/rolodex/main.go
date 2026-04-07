package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"strings"
)

//go:embed version.txt
var version string

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "merge":
		if err := runMerge(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "resolve":
		if err := runResolve(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "review":
		if err := runReview(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Printf("rolodex v%s\n", strings.TrimSpace(version))
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: rolodex <command> [flags]

Commands:
  merge    Merge and deduplicate vCard files
  review   Interactively review uncertain matches
  resolve  Apply review decisions from an edited report
  version  Print version

Run 'rolodex <command> -help' for details.
`)
}

func runMerge(args []string) error {
	fs := flag.NewFlagSet("merge", flag.ExitOnError)
	icloudPath := fs.String("icloud", "", "path to iCloud .vcf export")
	googlePath := fs.String("google", "", "path to Google .vcf export")
	outPath := fs.String("out", "merged.vcf", "output path for merged contacts")
	reviewPath := fs.String("review", "review.vcf", "output path for review-tier contacts")
	reportPath := fs.String("report", "", "output path for JSON report")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *icloudPath == "" || *googlePath == "" {
		return fmt.Errorf("both --icloud and --google flags are required")
	}

	return merge(*icloudPath, *googlePath, *outPath, *reviewPath, *reportPath)
}

func runReview(args []string) error {
	fs := flag.NewFlagSet("review", flag.ExitOnError)
	reportPath := fs.String("report", "", "path to report.json")
	reviewPath := fs.String("review", "", "path to review.vcf")
	calibrationPath := fs.String("calibration", "", "output path for calibration log (default: alongside report.json)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *reportPath == "" || *reviewPath == "" {
		return fmt.Errorf("--report and --review flags are required")
	}

	return reviewInteractive(*reportPath, *reviewPath, *calibrationPath)
}

func runResolve(args []string) error {
	fs := flag.NewFlagSet("resolve", flag.ExitOnError)
	reportPath := fs.String("report", "", "path to edited report.json")
	reviewPath := fs.String("review", "", "path to review.vcf")
	mergedPath := fs.String("merged", "", "path to merged.vcf")
	outPath := fs.String("out", "final.vcf", "output path for resolved contacts")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *reportPath == "" || *reviewPath == "" || *mergedPath == "" {
		return fmt.Errorf("--report, --review, and --merged flags are required")
	}

	return resolve(*reportPath, *reviewPath, *mergedPath, *outPath)
}
