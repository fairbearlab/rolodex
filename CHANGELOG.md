# Changelog

## \[0.3.0.0] - 2026-04-11

One command to merge, review, and resolve your contacts. Plus a new audit command to find contacts you can't actually reach.

### Added

* **`rolodex run`** unifies the entire merge-review-resolve pipeline into a single command. Run `rolodex run --icloud X --google Y` and the tool handles everything: merges contacts, launches the TUI for uncertain matches, resolves decisions, and writes a clean `final.vcf`. No intermediate files to manage.
* **`rolodex audit`** finds unreachable contacts missing both email and phone. Works on any VCF file. Text and JSON output formats. Parse warnings are surfaced so you know which entries were excluded.
* **`--include-names-only` flag** on `audit` flags contacts with only a name (no email, phone, or org) as low-signal noise worth removing.
* **`--keep` flag** on `run` preserves intermediate files (merged.vcf, review.vcf) alongside the output for debugging. Stale artifacts from prior runs are automatically cleaned up.
* **`--report` flag** on `run` saves the full report.json to a specified path.
* **Calibration data** appended alongside output when the review TUI runs, accumulating across sessions so threshold tuning suggestions improve over time.
* **Review pause detection.** Quitting the TUI with pending decisions preserves the workspace and prints resume instructions instead of treating it as an error.

### Changed

* Extracted `runPipeline()` from `pipeline.go` so both `merge` and `run` share the same core logic without duplication.
* `review.Run()` now returns `(bool, error)` indicating whether all clusters were resolved, enabling the pause detection above.
* Consolidated error handling in `main.go` to a single exit point.
* Updated CLI help text to list `run` as the recommended workflow.

### Fixed

* **`--keep` no longer overwrites accumulated calibration data.** Previously, the `--keep` copy used a full overwrite that destroyed multi-session calibration logs. Now calibration is handled exclusively via append.
* Path collision detection expanded: `--report` and `--out` are now checked against all reserved paths (calibration.jsonl, merged.vcf, review.vcf) to prevent silent overwrites.
* Report and intermediate file copies now use `0600` permissions (matching the reporter package) instead of world-readable `0644`.
* Output directory paths are validated before running the pipeline, preventing partial-success failures on bad `--report` paths.
* Removed `has_email` and `has_phone` fields from audit JSON output (they were structurally always `false` for unreachable contacts).

## \[0.2.0.0] - 2026-04-07

Interactive review command. Walk through uncertain matches one at a time in a terminal UI instead of hand-editing JSON.

### Added

* **`rolodex review`** **command** with BubbleTea TUI for interactively reviewing uncertain contact matches. Merge or skip with single keypresses, undo decisions, toggle between compact and detailed views.
* **Adaptive pacing** automatically shows a compact card for high-confidence pairs (score >= 0.78) and a full field-by-field diff with score breakdown for ambiguous pairs.
* **Calibration logging** records every decision with score, features, timing, and view mode to a JSONL file. End-of-session summary suggests threshold adjustments based on your actual decisions.
* **Per-feature score breakdown** shows why each pair scored the way it did: name similarity, shared email, shared phone, shared org with individual weights.
* **Shared report+review loader** extracted from resolve, used by both `resolve` and `review` commands.

### Changed

* Scorer now returns per-feature scores alongside the composite score, propagated through to report.json (backward-compatible addition).
* Report.json review entries include a `features` field with per-feature score data.

## \[0.1.0.0] - 2026-04-04

First working release. Merge and deduplicate vCard files from iCloud and Google exports with confidence scoring and explainable decisions.

### Added

* **vCard parser** reads iCloud and Google .vcf exports into a canonical contact model. Handles structured names, multi-value emails/phones, addresses, photos, and extra fields. Malformed entries produce warnings instead of aborting.
* **Contact normalizer** with Unicode NFKD + accent stripping, case folding, title/suffix removal for names. Digits-only phone normalization with US country code stripping.
* **Candidate blocker** groups contacts by shared email, phone, or last name to avoid N\*M comparisons. Large last-name blocks apply secondary filter.
* **Similarity scorer** with Jaro-Winkler name comparison and \~120 nickname mappings (Bob/Robert, Bill/William, etc.). Weighted scoring: name 0.40, email 0.25, phone 0.25, org 0.10.
* **Merger** with union-find clustering and pairwise validation. iCloud priority for single-value fields, union for emails/phones. Passthrough for fields present in only one source.
* **vCard 3.0 writer** with X-ROLODEX-SOURCE, X-ROLODEX-SCORE, and X-ROLODEX-REVIEW provenance extension fields.
* **JSON reporter** with summary stats, per-merge decisions with conflict details, review-tier ambiguities, and distinct entries.
* **Resolve command** applies user decisions from an edited report.json to review-tier contacts.
* **CLI** with `merge` and `resolve` subcommands.
* **CI/CD** with GitHub Actions for test-on-push and GoReleaser for cross-platform binary releases.
* **30+ tests** including unit tests, integration test, benchmark (1000 contacts in \~26ms), and fuzz targets.

