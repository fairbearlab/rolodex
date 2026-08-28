# Changelog

## \[0.4.0] - 2026-08-28

Duplicate detection that works on sparse real-world exports, and a review TUI that renders.

### Changed

* **Scoring rules for sparse contacts.** The linear score is still used for ranking, but rules now sit on top of the tier thresholds: an identical name (equal after normalization and nickname expansion) plus a shared phone, email or birthday is `auto_merge`; a near-identical name (similarity >= 0.95) on its own is floored at `review` instead of `distinct`; and two well-formed birthdays that disagree cap any pair at `review`, so a parent and child sharing a household phone are never merged unseen. Previously "same name + same phone" scored 0.65 and always needed a human, and "same name, nothing else" (0.40) was silently dropped. On the author's exports this moves auto-merges from 7 to 27 and the review queue from 4 to 42 clusters, out of 559 candidate pairs. As a consequence nickname pairs with a shared identifier (Bob/Robert Smith + same email) now auto-merge rather than land in review.
* **"Identical name" requires real name evidence.** Two contacts with no family name do not share one, and a single-letter given name is an initial, not a name, so `Alex` / `Alex` or `J. Smith` / `J. Smith` on a shared office phone go to review rather than auto-merging. Google folds the middle initial into the given name (`John V`) where iCloud uses the middle slot; the two shapes now compare as the same name instead of the bare `V` being read as a generational suffix.
* **Birthday is a scoring signal** (0.10 bonus, total capped at 1.0). When two agreeing birthdays are merged, the more specific wins: iCloud's `--10-22` (year omitted) is completed by Google's `1989-10-22` instead of the yearless form silently winning on source priority, in both the auto-merge path and `resolve`; `report.json` compares birthdays as dates too, so a pair merged on a shared birthday is no longer also reported as a `BDAY` conflict. `BDAY` is normalized at parse time so `1989-10-22`, `19891022`, `--1022`, Apple's placeholder year `1604`, and hand-typed forms such as `10/22/1989`, `22.10.1989`, `October 22, 1989` and `22 Oct 1989` compare correctly. Only a canonical date counts as evidence in either direction: equal free text (`1989`, `unknown`) is not a shared birthday, and a birthday that still cannot be read is treated as *unknown* — it is not a conflict, but the single-identifier exact-name rule does not fire, because the guard that rule depends on could not run. The review TUI shows why.
* **Same-name pairs are reviewed as pairs.** A pair that is in review on its name alone is a prompt for a human, not a link between people, so the merger no longer chains clusters through it. Before, union-find collapsed everyone sharing a common name into one review card — six unrelated "David Lee"s with six phones and six emails became one six-member cluster whose single merge action deleted five people. Near-name-only edges now pair two otherwise unattached contacts (most similar first, cross-source before same-source); a third namesake stays distinct instead of being stacked onto a cluster it has no tie to. Edges with a shared identifier chain as before.
* **Cluster ids are unique.** They hashed only source and name per member, so two unrelated "Alex" pairs shared an id and a decision on one was written onto both in `report.json`. The member index is now part of the hash. Ids from the same inputs are unchanged across re-runs, but a `report.json` written by an earlier version will not match a `review.vcf` written by this one — re-run `merge` rather than resuming.
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
* **Escaped separators and literal backslashes survive a parse/write cycle in every field.** `ORG:Acme\; Inc.` (an escaped semicolon inside a component) was split on the escape and written back as `Acme\\;Inc.`, which readers take as organization `Acme\` with a unit `Inc.`; `N:O\;Brien;Sean;;;` was read as family `O\`, given `Brien`, middle `Sean`; and a literal backslash before a separator (`N:Smith\\;John;;;`, family `Smith\`) was written back as `N:Smith\;John;;;`, one family name `Smith;John`. The root cause was upstream of the writer: go-vcard's decoder resolves `\\` but not `\;`, so the two inputs were identical by the time the parser saw them. The parser now takes the wire form and decodes each property itself — `N` and `ADR` split on unescaped separators only — and the writer escapes every value on the way out. `ORG` and unmodeled passthrough fields are kept in wire form and emitted verbatim.
* `rolodex review` refuses a `review.vcf` whose length does not match `report.json` instead of silently pairing later clusters with the wrong contacts. `resolve` already did; the TUI now fails the same way before a decision is recorded.
* A `"."` middle name (a common export placeholder) crashed the scorer; middle initials are now compared by rune, so `Ö` matches `Östen`.
* **A folded middle initial is no longer eaten as a suffix.** `v` sat in the suffix table, so `John V Doe` normalized to a bare `john` with no middle name — and an empty middle name is compatible with every other initial, so `John V Doe` and `John W Doe` sharing one email merged unseen. A single letter is an initial; a real generational `V` still counts in the dedicated `N` suffix component.
* **A set of initials is not a name.** The guard counted runes across the whole given name, so `J.R.` and `J R` slipped past the check that rejects `J`, and two different `J.R. Smith`s on one office switchboard auto-merged.
* **Diacritics distinguish people.** Names are folded for blocking and similarity, but the identical-name rule now also compares an accent-preserving form: `Nguyên` and `Nguyễn`, `Hà` and `Ha`, `René` and `Rene` go to review instead of merging on a shared phone alone. Compatibility variants are still one name — halfwidth kana and fullwidth Latin (a routine iCloud-vs-Google divergence) match as before.
* **Impossible birthdays are not evidence.** `1989-02-31`, `1999-02-29` and year `0000` passed the month and day range check and counted as a shared birthday, which is enough to auto-merge two same-named contacts with no phone or email. Only real calendar dates count now; `2000-02-29` and a no-year `--02-29` still do.
* **A stale `review.vcf` with the right number of contacts is refused.** The length check passed and every cluster was then handed the wrong people, recording the decision against another cluster's id. Each contact's `X-ROLODEX-CLUSTER` is now checked against the report, as `resolve` already did.
* **Control characters are stripped from every field at parse time.** A contact name carrying terminal escapes could clear and repaint the review screen, and a right-to-left override reordered the *other* card on the same row — hiding what the reviewer was about to merge. A bare carriage return also survived into the written `.vcf`, where it could forge a property line. Zero-width joiners are kept: they are load-bearing in Indic and Persian names and in emoji.
* **`merge` refuses to write two outputs to one file, or over an input.** `--out dir/review.vcf` aimed the merged output and the review set at the same path and the merged contacts were lost; on macOS, where the filesystem is case-insensitive by default, `--out Merged.vcf --review merged.vcf` deleted the merged file outright and still exited 0. Writing `--out` over `--icloud` destroyed the source export. All are now rejected before anything is written. `run` and `resolve` apply the same guard: `run --out icloud.vcf` replaced the iCloud export with the resolved output (the pipeline reads both inputs before it writes, so nothing failed), and `resolve --out merged.vcf` did the same to its `--merged` input.
* **A derived `review.vcf` is only deleted when rolodex wrote it.** When a run produced nothing to review it removed the review path — which now defaults next to `--out`, so `merge --out ~/Documents/merged.vcf` deleted `~/Documents/review.vcf`, a file rolodex never wrote. Skipping the removal for every derived path went too far the other way: a re-run after fixing an export left the previous run's `review.vcf` beside a `report.json` that said there was nothing to review, and `resolve` refused the pair as misaligned. A file at the derived path is now removed only if its cards carry `X-ROLODEX-REVIEW`; a path named with `--review` is cleared as before.
* **Malformed entries are reported.** A truncated card was skipped and its contact silently absent from every output, with the "contacts loaded" count taken after the loss. `merge` and `run` now name the skipped entries on stderr, as `audit` already did.
* Organizations containing a literal semicolon render as `Acme; Inc.` in the review card instead of `Acme\, Inc.` — the card is what a merge decision is made on.
* **A placeholder is not a shared identifier.** Any two equal strings counted as a shared phone or email, so two contacts both carrying `TEL:0`, `000-000-0000` or `EMAIL:unknown` were treated as confirmed and auto-merged on the name alone. A phone now needs seven digits that are not all the same, an email needs a local part and a dotted domain, and a birthday must not be a January 1st: `1970-01-01`, `1900-01-01`, `2000-01-01` and Apple's `1604-01-01` are what an export carries when nobody entered a date, and two same-named contacts sharing one (different phones, different emails) scored 0.500 and auto-merged. A shared January 1st now counts as no birthday at all — neither evidence nor a conflict — and the pair goes to review.
* **A birthday cannot smuggle in trailing text.** The ISO pattern accepted anything after the date, so `1989-10-22 or 23` was read as a firm `1989-10-22`. Only a real time suffix is allowed now; `1989-10-22T00:00:00Z` and `1989-10-22 00:00:00` still normalize.
* `merge` resolves symlinks before comparing paths, so an input reached through a symlinked directory can no longer be overwritten by an output named directly.
* **One definition of a valid birthday.** The scorer repeated the normalizer's month and day bounds instead of calling it, and the copy did not learn about real calendar dates: `1989-02-31` was rejected by the normalizer and then accepted by the scorer as a shared birthday, enough to auto-merge two same-named contacts with no phone or email. Both now go through `normalize.ParseCanonicalBirthday`.
* Review cards no longer tick a placeholder as a shared value. The score said the pair was unconfirmed while the card showed a green check beside `000-000-0000` on both sides — opposite cues on the screen where the merge is chosen.
* `rolodex version` reports the release it was built from. `cmd/rolodex/version.txt` is embedded into the binary and had drifted from `VERSION`; `make version-check` now fails the build if the two disagree.

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

