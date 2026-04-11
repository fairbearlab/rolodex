# Architecture

Rolodex is a Go CLI tool that merges and deduplicates vCard 3.0 files exported from iCloud and Google Contacts. It follows a serial pipeline architecture with each stage in its own package under `internal/`. The CLI entry point in `cmd/rolodex/` orchestrates the pipeline.

## Directory layout

```text
cmd/rolodex/
  main.go              CLI entry point, command routing
  pipeline.go          Pipeline orchestration for merge/review/resolve
internal/
  model/               Shared data types (ParsedContact, ScoredPair, Report, etc.)
  parser/              vCard 3.0 parsing
  normalize/           Name, email, phone normalization
  blocker/             Candidate pair generation
  scorer/              Similarity scoring and tier classification
  merger/              Union-find clustering and contact merging
  writer/              vCard 3.0 output with provenance fields
  reporter/            JSON report generation
  review/              Interactive TUI for uncertain matches
  resolve/             Apply review decisions, write final output
  calibration/         Decision logging and threshold analysis
testdata/              Test fixtures (icloud.vcf, google.vcf)
```

## Data flow

The tool has three commands that form a pipeline:

```text
                        merge command
                        ============

  icloud.vcf ──┐
                ├─→ Parse → Normalize → Block → Score → Merge ─┬─→ merged.vcf
  google.vcf ──┘                                                ├─→ review.vcf
                                                                └─→ report.json


                       review command
                       ==============

  report.json ──┐
                ├─→ TUI (BubbleTea) ─┬─→ report.json (updated decisions)
  review.vcf  ──┘                    └─→ calibration.jsonl


                       resolve command
                       ===============

  report.json ──┐
  review.vcf  ──┼─→ Apply decisions ──→ final.vcf
  merged.vcf  ──┘
```

**Typical workflow:** `merge` produces three files. `review` walks through uncertain matches interactively. `resolve` combines the confident merges with reviewed decisions into a single output file.

## Core types

All types live in `internal/model/`. The main chain:

```text
ParsedContact → NormalizedContact → ScoredPair → MergedContact
```

| Type | Purpose |
| --- | --- |
| `ParsedContact` | Raw vCard entry: structured name, emails, phones, addresses, photo, plus `Extra` catch-all map for unmodeled fields |
| `NormalizedContact` | Wraps a `ParsedContact` with normalized names (NFKD + case fold), emails (lowercase), and phones (digits-only) |
| `ScoredPair` | A candidate match between two contacts with a composite score, per-feature breakdown (`ScoreFeatures`), and tier classification |
| `MergedContact` | Final output contact with source provenance, score, and review flag |
| `Cluster` | A group of contacts connected by scored pairs (used by union-find) |
| `Tier` | Classification enum: `auto_merge` (>= 0.85), `review` (0.60-0.85), `distinct` (< 0.60) |
| `Report` | JSON report structure: summary stats, merge decisions, review decisions, distinct entries, warnings |

## Package details

### parser

Reads vCard 3.0 files using `emersion/go-vcard`. Extracts structured name components (N field), formatted name (FN), emails, phones, org, title, birthday, addresses, notes, URLs, and photos. Unmodeled vCard properties are stored in `Extra` for lossless round-tripping. Malformed entries produce warnings instead of aborting the parse.

### normalize

Prepares contacts for comparison. Names go through Unicode NFKD decomposition, accent/combining-mark stripping, case folding, whitespace collapse, and title/suffix removal (Dr., Jr., III, etc.). Phones are reduced to digits-only with US country code stripping (11-digit numbers starting with 1). Emails are lowercased and trimmed.

### blocker

Generates candidate pairs for scoring without comparing every contact against every other (avoids N*M). Three blocking keys: shared normalized email, shared normalized phone, shared normalized last name. Last-name blocks larger than 50 contacts get a secondary filter — only pairs with matching first initials or shared org are retained.

### scorer

Computes a weighted composite score for each candidate pair:

- **Name similarity** (0.40): Jaro-Winkler distance on full name strings, with nickname expansion (~120 mappings like Bob/Robert, Bill/William). The higher of direct and nickname-expanded scores is used.
- **Shared email** (0.25): Binary — any normalized email in common.
- **Shared phone** (0.25): Binary — any normalized phone in common.
- **Shared org** (0.10): Exact match on lowercased org field.

Contacts missing a given name use adjusted weights (0.45/0.45/0.10) and require 2+ matching identifiers for auto-merge.

Pairs are classified into tiers by score threshold.

### merger

Clusters connected contacts using a union-find (disjoint set) data structure. Two contacts in the same cluster are transitively related — if A matches B and B matches C, all three form one cluster.

Before auto-merging a cluster, every internal pair is validated. If any pair is review-tier, distinct, or unscored (not blocked together), the entire cluster is demoted to review.

Merge logic for auto-merge clusters uses **iCloud priority**: single-value fields (name, title, org, birthday, note, URL, photo) take the iCloud value, falling through to Google if iCloud is empty. Multi-value fields (emails, phones, addresses) are unioned with deduplication by normalized value. Extra fields are unioned by key.

### writer

Encodes `MergedContact` slices as vCard 3.0 using `emersion/go-vcard`. Adds provenance extension fields: `X-ROLODEX-SOURCE`, `X-ROLODEX-SCORE`, `X-ROLODEX-REVIEW`. File writes are atomic (write to temp file, fsync, rename) to prevent corrupt output on crash.

### reporter

Generates a JSON report (`model.Report`) with:
- **Summary**: contact counts per source, auto-merged/review/distinct/warning counts.
- **Merged**: per-cluster decisions with score, contributing contacts, field conflicts, and result name.
- **Review**: per-cluster decisions with score, per-feature breakdown, ambiguity explanation, and decision status (pending/merge/skip).
- **Distinct**: singletons not matched to anything.
- **Warnings**: parse errors with source and index.

### review

Interactive terminal UI built on BubbleTea. Loads review clusters from `report.json` and `review.vcf`, sorted by score descending. Adaptive pacing shows a compact card for high-confidence pairs (score >= 0.78) and a full field-by-field diff for ambiguous pairs. Supports merge (`m`), skip (`s`), undo (`u`), detail toggle (`d`), and quit (`q`). Decisions are saved to `report.json` after every keypress, so sessions can be interrupted and resumed.

### resolve

Reads `report.json`, `review.vcf`, and `merged.vcf`. For each review cluster: if the decision is "merge", merges the cluster contacts; if "skip", excludes them; if "pending", keeps all contacts as-is. Combines with the auto-merged contacts and writes `final.vcf`.

### calibration

Logs every review decision to a JSONL file with cluster ID, score, per-feature scores, view mode, decision, and response time. At the end of a review session, analyzes the log to suggest threshold adjustments — e.g., if the user merged everything above 0.78, suggests raising the auto-merge threshold accordingly.

## Key design decisions

**iCloud priority for single-value fields.** When both sources have a value for a field like name or title, the iCloud version wins. This assumes iCloud is the user's primary contact store. Multi-value fields are unioned instead since both sources may have unique entries.

**Union-find for clustering.** Handles transitive matches naturally — if A matches B and B matches C, they form one cluster without explicitly comparing A to C. This is important because the blocker may not have paired A and C directly.

**Pairwise validation before auto-merge.** A cluster only auto-merges if every internal pair scores at or above the auto-merge threshold. One weak link (review-tier pair, or an unscored pair that wasn't blocked together) demotes the entire cluster to review. This prevents false merges in chains like A-B (strong) + B-C (strong) where A-C might be unrelated.

**Blocking before scoring.** Scoring is the most expensive stage (Jaro-Winkler on every pair). Blocking by shared email, phone, or last name reduces the candidate set from O(N*M) to a small fraction, keeping the tool fast on large address books (~26ms for 1000 contacts).

**Atomic file writes.** Output is written to a temp file, fsynced, then renamed over the target. This prevents corrupt output if the process crashes mid-write.

**Provenance extension fields.** Output vCards carry `X-ROLODEX-SOURCE`, `X-ROLODEX-SCORE`, and `X-ROLODEX-REVIEW` headers so the merge history is traceable in the output file itself.

## Dependencies

| Package | Purpose |
| --- | --- |
| `emersion/go-vcard` | vCard 3.0 parsing and encoding |
| `xrash/smetrics` | Jaro-Winkler string distance |
| `golang.org/x/text` | Unicode NFKD normalization |
| `charmbracelet/bubbletea` | Terminal UI framework (review command) |
| `charmbracelet/lipgloss` | Terminal styling (review command) |

## Testing

Each package has unit tests. Key testing approaches:

- **Fuzz targets** in `parser/` and `normalize/` for robustness against malformed input.
- **Integration test** in `cmd/rolodex/` runs the full merge pipeline against test fixtures and validates output structure.
- **Benchmark** in `cmd/rolodex/` measures end-to-end performance with 1000 synthetic contacts (~26ms).
- **Test fixtures** in `testdata/` provide realistic iCloud and Google vCard exports with overlapping contacts, nickname variations, and different phone formats.
