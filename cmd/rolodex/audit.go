package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fairbearlab/rolodex/internal/audit"
	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/parser"
)

func runAuditCmd(vcfPath, format string, includeNamesOnly bool) error {
	contacts, warnings, err := parser.ParseFile(vcfPath, model.SourceUnknown)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", vcfPath, err)
	}

	result := audit.Audit(contacts, audit.AuditOptions{
		IncludeNamesOnly: includeNamesOnly,
	})
	result.Warnings = warnings

	switch format {
	case "json":
		return printAuditJSON(result)
	case "text":
		printAuditText(result)
		return nil
	default:
		return fmt.Errorf("unknown format %q (use text or json)", format)
	}
}

func printAuditJSON(result audit.AuditResult) error {
	type jsonOutput struct {
		Total            int                        `json:"total"`
		UnreachableCount int                        `json:"unreachable_count"`
		Unreachable      []audit.UnreachableContact `json:"unreachable"`
		WarningCount     int                        `json:"warning_count"`
		Warnings         []model.Warning            `json:"warnings"`
	}
	out := jsonOutput{
		Total:            result.Total,
		UnreachableCount: result.UnreachableCount,
		Unreachable:      result.Unreachable,
		WarningCount:     len(result.Warnings),
		Warnings:         result.Warnings,
	}
	if out.Unreachable == nil {
		out.Unreachable = []audit.UnreachableContact{}
	}
	if out.Warnings == nil {
		out.Warnings = []model.Warning{}
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func printAuditText(result audit.AuditResult) {
	fmt.Println()
	fmt.Println("Contact Quality Audit")
	fmt.Println(strings.Repeat("\u2501", 39))
	fmt.Println()

	if len(result.Warnings) > 0 {
		fmt.Printf("Parse warnings: %d (malformed entries excluded from audit)\n", len(result.Warnings))
		for _, w := range result.Warnings {
			fmt.Printf("  - entry %d: %s\n", w.Index, w.Message)
		}
		fmt.Println()
	}

	fmt.Printf("Total contacts: %d\n", result.Total)
	fmt.Printf("Unreachable (no email, no phone): %d\n", result.UnreachableCount)

	if result.UnreachableCount == 0 {
		fmt.Println("\nAll contacts have at least an email or phone number.")
		return
	}

	fmt.Println()
	fmt.Println("Unreachable contacts:")
	for i, c := range result.Unreachable {
		has := describeHas(c)
		fmt.Printf("  %d. %s — has: %s\n", i+1, c.Name, has)
	}

	fmt.Printf("\n%d contacts have no way to reach them.\n", result.UnreachableCount)
	fmt.Println("Consider removing or enriching these contacts.")
}

func describeHas(c audit.UnreachableContact) string {
	var parts []string
	if c.HasOrg {
		parts = append(parts, "org")
	}
	if c.HasTitle {
		parts = append(parts, "title")
	}
	if c.HasAddress {
		parts = append(parts, "address")
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, ", ")
}

