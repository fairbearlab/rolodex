package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed version.txt
var version string

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "run":
		err = runRunCmd(os.Args[2:])
	case "merge":
		err = runMerge(os.Args[2:])
	case "review":
		err = runReview(os.Args[2:])
	case "resolve":
		err = runResolve(os.Args[2:])
	case "audit":
		err = runAuditCmdFlags(os.Args[2:])
	case "version":
		fmt.Printf("rolodex v%s\n", strings.TrimSpace(version))
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
	if err != nil {
		if errors.Is(err, ErrReviewPaused) {
			// Review pause is not an error — the user intentionally quit
			// with pending decisions. Messages already printed by run().
			return
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: rolodex <command> [flags]

Commands:
  run      Merge, review, and resolve contacts in one step
  merge    Merge and deduplicate vCard files (individual step)
  review   Interactively review uncertain matches (individual step)
  resolve  Apply review decisions from an edited report (individual step)
  audit    Find unreachable contacts missing email and phone
  version  Print version

Run 'rolodex <command> -help' for details.
`)
}

func runRunCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	icloudPath := fs.String("icloud", "", "path to iCloud .vcf export")
	googlePath := fs.String("google", "", "path to Google .vcf export")
	outPath := fs.String("out", "final.vcf", "output path for final resolved contacts")
	reportPath := fs.String("report", "", "save report.json to this path")
	keep := fs.Bool("keep", false, "keep intermediate files alongside output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *icloudPath == "" || *googlePath == "" {
		return fmt.Errorf("both --icloud and --google flags are required")
	}

	return run(*icloudPath, *googlePath, *outPath, *reportPath, *keep)
}

func runAuditCmdFlags(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	format := fs.String("format", "text", "output format: text or json")
	namesOnly := fs.Bool("include-names-only", false, "also flag contacts with only a name")

	// Reorder args so flags precede the positional file path.
	// flag.FlagSet stops at the first non-flag token, so
	// "audit file.vcf --format json" would silently ignore --format.
	reordered := reorderFlagsBeforePositional(args, fs)

	if err := fs.Parse(reordered); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: rolodex audit <file.vcf>")
	}

	return runAuditCmd(fs.Arg(0), *format, *namesOnly)
}

// reorderFlagsBeforePositional moves flag arguments (and their values) before
// positional arguments so that flag.FlagSet.Parse works regardless of order.
func reorderFlagsBeforePositional(args []string, fs *flag.FlagSet) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}
		flags = append(flags, a)
		if strings.Contains(a, "=") {
			continue
		}
		name := strings.TrimLeft(a, "-")
		if f := fs.Lookup(name); f != nil {
			if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
				continue
			}
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		}
	}
	return append(flags, positional...)
}

func runMerge(args []string) error {
	fs := flag.NewFlagSet("merge", flag.ExitOnError)
	icloudPath := fs.String("icloud", "", "path to iCloud .vcf export")
	googlePath := fs.String("google", "", "path to Google .vcf export")
	outPath := fs.String("out", "merged.vcf", "output path for merged contacts")
	reviewPath := fs.String("review", "", "output path for review-tier contacts (default: review.vcf next to --out)")
	reportPath := fs.String("report", "", "output path for JSON report")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *icloudPath == "" || *googlePath == "" {
		return fmt.Errorf("both --icloud and --google flags are required")
	}
	reviewDerived := *reviewPath == ""
	if reviewDerived {
		*reviewPath = filepath.Join(filepath.Dir(*outPath), "review.vcf")
	}
	inputs := []pathFlag{{"--icloud", *icloudPath}, {"--google", *googlePath}}
	outputs := []pathFlag{{"--out", *outPath}, {"--review", *reviewPath}, {"--report", *reportPath}}
	if err := checkDistinctPaths(inputs, outputs); err != nil {
		return err
	}

	return merge(*icloudPath, *googlePath, *outPath, *reviewPath, *reportPath, reviewDerived)
}

// pathFlag names a path the way the user gave it, for error messages.
type pathFlag struct{ flag, path string }

// checkDistinctPaths rejects a command whose paths collide: an output over
// an input, or two outputs on one file. --review defaults to a path derived
// from --out, so "merge --out dir/review.vcf" aimed both writes at one file:
// the merged contacts were written and then overwritten by the review set,
// with no error. run had a narrower copy of this check that never included
// its inputs, so "run --out icloud.vcf" replaced the iCloud export with the
// resolved output and exited 0; resolve had no check at all.
//
// Three subtleties, each of which cost a whole address book in testing:
//   - Keys are case-folded. macOS APFS and HFS+ are case-insensitive by
//     default, so "--out Merged.vcf --review merged.vcf" is ONE file that
//     filepath.Abs reports as two. The stale-review removal then unlinked the
//     merged output and the command still exited 0.
//   - The input exports are included. Writing --out over --icloud destroyed
//     the one artifact a user needs if the merge went wrong.
//   - writer.WriteFile stages through "<path>.tmp", so that sibling is
//     reserved too.
//
// Empty paths (an optional flag left unset) are skipped.
func checkDistinctPaths(inputs, outputs []pathFlag) error {
	seen := make(map[string]string)
	claim := func(key, flag string) error {
		if prev, ok := seen[key]; ok && prev != flag {
			return fmt.Errorf("%s and %s refer to the same file; they must be different", prev, flag)
		}
		seen[key] = flag
		return nil
	}

	for _, e := range append(append([]pathFlag{}, inputs...), outputs...) {
		if e.path == "" {
			continue
		}
		k, err := pathKey(e.path)
		if err != nil {
			return fmt.Errorf("resolving %s path %q: %w", e.flag, e.path, err)
		}
		if err := claim(k, e.flag); err != nil {
			return err
		}
	}
	// Reserve the staging sibling of every file we write, and refuse to write
	// on top of an input.
	for _, e := range outputs {
		if e.path == "" {
			continue
		}
		k, err := pathKey(e.path + ".tmp")
		if err != nil {
			return fmt.Errorf("resolving %s path %q: %w", e.flag, e.path, err)
		}
		if err := claim(k, e.flag); err != nil {
			return err
		}
	}
	return nil
}

// pathKey identifies the file a path names, resolving symlinks as well as
// case. filepath.Abs cleans a path but follows nothing, so "--icloud
// real/icloud.vcf --out alias/icloud.vcf" through a symlinked directory
// looked like two files and the source export was overwritten. A path that
// does not exist yet still has a parent that usually does, so resolve the
// directory and keep the base name.
func pathKey(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return strings.ToLower(resolved), nil
	}
	dir, base := filepath.Split(abs)
	if resolvedDir, err := filepath.EvalSymlinks(filepath.Clean(dir)); err == nil {
		return strings.ToLower(filepath.Join(resolvedDir, base)), nil
	}
	return strings.ToLower(abs), nil
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
	inputs := []pathFlag{{"--report", *reportPath}, {"--review", *reviewPath}, {"--merged", *mergedPath}}
	if err := checkDistinctPaths(inputs, []pathFlag{{"--out", *outPath}}); err != nil {
		return err
	}

	return resolve(*reportPath, *reviewPath, *mergedPath, *outPath)
}
