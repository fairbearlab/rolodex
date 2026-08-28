# Changelog

## \[Unreleased]

Duplicate detection that works on sparse real-world exports, and a review TUI that renders.

### Changed

* **Scoring rules for sparse contacts.** The linear score is still used for ranking, but rules now sit on top of the tier thresholds: an identical name (equal after normalization and nickname expansion) plus a shared phone, email or birthday is `auto_merge`; a near-identical name (similarity >= 0.95) on its own is floored at `review` instead of `distinct`; and two well-formed birthdays that disagree cap any pair at `review`, so a parent and child sharing a household phone are never merged unseen. Previously "same name + same phone" scored 0.65 and always needed a human, and "same name, nothing else" (0.40) was silently dropped. On the author's exports this moves auto-merges from 7 to 26 and the review queue from 4 to 36 clusters, out of 559 candidate pairs. As a consequence nickname pairs with a shared identifier (Bob/Robert Smith + same email) now auto-merge rather than land in review.
* **Birthday is a scoring signal** (0.10 bonus, total capped at 1.0). `BDAY` is normalized at parse time so `1989-10-22`, `19891022`, `--1022` and Apple's placeholder year `1604` compare correctly.
* **Compact review card threshold** lowered from 0.78 to 0.60. Pairs at 0.60+ have a shared identifier and get the one-glance card; near-name-only pairs get the full diff. Pairs held in review by a birthday conflict can score up to 1.00 and always get the full diff, which is the only view that shows the birthdays.
* `rolodex merge` defaults `--review` to `review.vcf` next to `--out` instead of the current directory.
* `liam`, `jack`, `jamie`, `leo` and `harry` removed from the nickname table — they are standalone given names, and mapping them to a canonical made Will/Liam or Jack/John pairs look identical. Two different diminutives of one canonical (Ted/Ned, Beth/Betty) no longer count as an identical name either, and neither do differing middle names or generational suffixes (John A. Smith / John B. Smith, John Smith Jr. / Sr.).

### Fixed

* **Review TUI layout.** Side-by-side cards were 2 columns wider than their container, so every line hard-wrapped into interleaved borders and blank lines. Widths are now derived from the lipgloss border/padding semantics, the 72-column cap is raised to 120, and the title no longer wraps `Score:` onto its own line.
* **Review cards are labelled `icloud` / `google`** (the iCloud card is on the left and marked as the conflict winner) instead of both reading `review`. The parser restores `Source` from `X-ROLODEX-SOURCE`, with the report's provenance as a fallback.
* Review progress counter now tracks the current cluster rather than the resolved count, so it is right after undo and on a partially-resolved report.
* Score-breakdown weight column aligns across rows; phones are displayed in one canonical form and values shared between the two cards are marked.
* The detailed view's save-error line was built after the content and never shown.
* Review card truncation measures terminal columns, so CJK and emoji names no longer overflow the card and wrap.
* `X-ROLODEX-SOURCE` is only consulted on read-back paths (review/resolve/audit); on `merge` the `--icloud`/`--google` flag stays authoritative even if a re-exported file carries the field.
* iCloud's `ORG:Acme;` (empty structured unit) no longer produces a false `ORG` conflict against Google's `ORG:Acme`, and no longer prevents org matching. Same for `BDAY` values that differed only by format.

## \[0.3.0] - 2026-04-11

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

## \[0.2.0] - 2026-04-07

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

## \[0.1.0] - 2026-04-04

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

