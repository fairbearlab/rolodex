# Architecture

Rolodex is a Go CLI tool that merges and deduplicates vCard 3.0 files exported from iCloud and Google Contacts. It follows a serial pipeline architecture with each stage in its own package under `internal/`. The CLI entry point in `cmd/rolodex/` orchestrates the pipeline.

## Directory layout

```text
cmd/rolodex/
  main.go              CLI entry point, command routing
  pipeline.go          Core merge pipeline (runPipeline), shared by merge and run
  run.go               Unified workflow: merge → review → resolve in one command
  audit.go             Contact quality audit (find unreachable contacts)
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
  audit/               Contact quality checks (reachability)
testdata/              Test fixtures (icloud.vcf, google.vcf)
```

## Data flow

The tool has five commands. The `run` command is the recommended workflow, wrapping merge/review/resolve into a single invocation:

```text
                          run command (unified)
                          =====================

  icloud.vcf ──┐
                ├─→ runPipeline() ─→ temp dir ─→ TUI (if review pairs) ─→ resolve ─→ final.vcf
  google.vcf ──┘                                                          └─→ calibration.jsonl

                          audit command
                          =============

  any .vcf ──→ Parse ──→ Check reachability ──→ text/json report


                     individual commands (merge → review → resolve)
                     ==============================================

  icloud.vcf ──┐
                ├─→ Parse → Normalize → Block → Score → Merge ─┬─→ merged.vcf
  google.vcf ──┘                                                ├─→ review.vcf
                                                                └─→ report.json

  report.json ──┐
                ├─→ TUI (BubbleTea) ─┬─→ report.json (updated decisions)
  review.vcf  ──┘                    └─→ calibration.jsonl

  report.json ──┐
  review.vcf  ──┼─→ Apply decisions ──→ final.vcf
  merged.vcf  ──┘
```

**`run` command:** Manages a temp directory for intermediates, calls `runPipeline()` for the core merge logic, launches the TUI if there are review-tier pairs, then resolves automatically. Calibration data is saved alongside the output. The `--keep` flag preserves intermediates.

**`audit` command:** Works on any VCF file independently. Flags contacts with no email and no phone as unreachable.

**Individual commands:** `merge`, `review`, and `resolve` are still available for users who want granular control. Both `merge` and `run` call the shared `runPipeline()` function.

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
| `Tier` | Classification enum: `auto_merge` (>= 0.85, or identical name + shared phone/email/birthday; never with conflicting birthdays, and not on one identifier when a birthday is present but unreadable), `review` (0.60-0.85, or a near-identical name alone; a birthday conflict caps a pair here but never raises it), `distinct` (< 0.60) |
| `Report` | JSON report structure: summary stats, merge decisions, review decisions, distinct entries, warnings |

## Package details

### parser

Reads vCard 3.0 files using `emersion/go-vcard`. Extracts structured name components (N field), formatted name (FN), emails, phones, org, title, birthday, addresses, notes, URLs, and photos. `ORG` is cleaned of empty trailing structured components (iCloud emits `Acme;`; a leading `;Dept` keeps its position) and `BDAY` is canonicalized to `YYYY-MM-DD` or `--MM-DD` (Google's `19891022`, iCloud's `X-APPLE-OMIT-YEAR`, the Apple placeholder year `1604`, and hand-typed slash, dotted and month-name forms are all recognized; anything else passes through untouched and the scorer treats it as unreadable). An escaped `\;` inside `ORG` is kept as part of its component. A `X-ROLODEX-SOURCE` of `icloud`/`google` written by an earlier run is restored into `Source` on read-back paths (review, resolve, audit); on `merge` the `--icloud`/`--google` flag is authoritative and the field is ignored. Unmodeled vCard properties are stored in `Extra` for lossless round-tripping. Malformed entries produce warnings instead of aborting the parse.

### normalize

Prepares contacts for comparison. Names go through Unicode NFKD decomposition, accent/combining-mark stripping, case folding, whitespace collapse, and title/suffix removal (Dr., Jr., III, etc.). Phones are reduced to digits-only with US country code stripping (11-digit numbers starting with 1). Emails are lowercased and trimmed.

### blocker

Generates candidate pairs for scoring without comparing every contact against every other (avoids N*M). Three blocking keys: shared normalized email, shared normalized phone, shared normalized last name. Last-name blocks larger than 50 contacts get a secondary filter — only pairs with matching first initials or shared org are retained.

### scorer

Computes a weighted composite score for each candidate pair:

- **Name similarity** (0.40): Jaro-Winkler distance on full name strings, with nickname expansion (~120 mappings like Bob/Robert, Bill/William). The higher of direct and nickname-expanded scores is used.
- **Shared email** (0.25): Binary — any normalized email in common.
- **Shared phone** (0.25): Binary — any normalized phone in common.
- **Shared org** (0.10): Exact match on lowercased org field (the parser has already dropped iCloud's empty trailing `;` component).
- **Shared birthday** (0.10, bonus; total capped at 1.0): Equal canonical `YYYY-MM-DD`, or a no-year `--MM-DD` matching the month and day of a full date. Both sides must be canonical, in-range dates; equal free text is not a match.

Contacts missing a given name use adjusted weights (0.45/0.45/0.10/0.10) and require 2+ matching identifiers for auto-merge.

Pairs are classified into tiers by score threshold, with rules layered on top (`scorer.Classify`) because real exports are sparse and the linear score rarely reaches 0.85 on its own: an identical name (`NameExact`: given and family equal, or one a nickname of the other; a family name present; a given name longer than an initial; compatible middle names, with trailing given-name tokens read as the middle name when the middle slot is empty; equal generational suffixes) plus a shared phone, email or birthday is `auto_merge`; a near-identical name (Jaro-Winkler >= 0.95, which also admits Eric/Erica) alone is floored at `review` so same-name pairs are surfaced to a human rather than dropped; and two well-formed birthdays that disagree (`BirthdayConflict`) cap any pair at `review`. When both contacts carry a birthday but one is unreadable (`BirthdayUnknown`), the conflict check cannot run, so the single-identifier exact-name rule does not fire and the pair falls through to the score thresholds — the guard fails closed.

### merger

Clusters connected contacts using a union-find (disjoint set) data structure. Two contacts in the same cluster are transitively related — if A matches B and B matches C, all three form one cluster. Transitivity applies only to edges that carry evidence (a shared identifier, or a score at the review threshold). A pair that is in review on its name alone is applied afterwards and only between two contacts nothing else has claimed, so same-name contacts are reviewed as pairs rather than collapsed into one cluster per common name; a third namesake with no tie stays distinct. Cluster ids hash each member's source, index and name, so they are unique within a run and stable across re-runs of the same inputs.

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

Interactive terminal UI built on BubbleTea. Loads review clusters from `report.json` and `review.vcf`, sorted by score descending. Adaptive pacing shows a compact card for pairs whose score reaches the review threshold (>= 0.60, meaning a shared phone or email backs the name match) and a full field-by-field diff, with the iCloud card on the left labelled as the conflict winner, for pairs surfaced by the exact-name rule alone. Supports merge (`m`), skip (`s`), undo (`u`), detail toggle (`d`), and quit (`q`). Decisions are saved to `report.json` after every keypress, so sessions can be interrupted and resumed.

### resolve

Reads `report.json`, `review.vcf`, and `merged.vcf`. For each review cluster: if the decision is "merge", merges the cluster contacts; if "skip", excludes them; if "pending", keeps all contacts as-is. Combines with the auto-merged contacts and writes `final.vcf`.

### calibration

Logs every review decision to a JSONL file with cluster ID, score, per-feature scores, view mode, decision, and response time. At the end of a review session, analyzes the log to suggest threshold adjustments — e.g., if the user merged everything above 0.78, suggests raising the auto-merge threshold accordingly.

### audit

Checks contact quality by scanning for unreachable contacts (no email and no phone). Takes `[]model.ParsedContact` and returns `AuditResult` with a list of `UnreachableContact` entries, each annotated with what the contact does have (org, title, address). Used by the `audit` CLI command.

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
