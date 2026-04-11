# TODOS

## Merge Engine

### Calibration dataset for scoring thresholds

**What:** Create a labeled set of 50-100 contact pairs from real exports, marked as match/non-match. Validate the auto_merge (0.85) and review (0.60) thresholds against real data.

**Why:** Thresholds are reasonable starting points from entity resolution prior art, but every contact dataset is different. Real validation is the only way to know if the scoring model actually prevents silent wrong merges.

**Context:** The design doc's success criteria already calls for this ("verified by manually labeling 50-100 contact pairs"). Requires real contact data (PII), so the labeled set should be anonymized or kept in a private, gitignored directory. Run after scorer is implemented. Compare precision at auto_merge threshold, recall at review threshold.

**Effort:** M
**Priority:** P1
**Depends on:** Scorer implementation

### Per-field provenance tracking

**What:** For merged contacts, track which source each field came from. Example: in report.json, `"name": {"value": "Bob Smith", "source": "icloud"}, "emails": [{"value": "bob@gmail.com", "source": "google"}]`.

**Why:** Contact-level provenance (X-ROLODEX-SOURCE:merged(icloud+google)) tells you the contact was merged but not which source won each field. Per-field provenance enables future features like rollback and makes the report much richer for manual review.

**Context:** Phase 1 has contact-level provenance + per-merge explanations in report.json. Per-field provenance is the natural next step. Adds complexity to the merge and report stages. The report.json already captures some of this in the decision explanations, so there's partial overlap to reconcile.

**Effort:** M
**Priority:** P3
**Depends on:** Merge stage must be stable first

## Review UX

### Re-merge detection

**What:** Add a guard to the merge command that detects an existing report.json with non-pending decisions before overwriting.

**Why:** Silent loss of review progress is the worst failure mode in the Phase 2 flow. If the user runs `rolodex merge` again while a review session is in progress, report.json gets overwritten and all decisions are lost with no warning.

**Context:** Both review and resolve commands depend on report.json. The simplest guard: merge checks if report.json exists, parses it, and if any ReviewDecision has decision != "pending", prompts "Report has N reviewed decisions. Overwrite? [y/N]". A `--force` flag bypasses the prompt.

**Effort:** S
**Priority:** P2
**Depends on:** Phase 2 shipped

### Threshold override flags on merge and run commands

**What:** Add `--auto-merge-threshold` and `--review-threshold` flags to `rolodex merge` and `rolodex run` to override the hardcoded 0.85/0.60 thresholds.

**Why:** Phase 2's calibration suggestions tell the user "your effective threshold should be X" but there's no way to act on it. These flags close the feedback loop. The `run` command should inherit them when `merge` gets them.

**Context:** ThresholdAutoMerge=0.85 and ThresholdReview=0.60 are constants in model/contact.go:96-98. Flags would override them per-run. The calibration end-of-session summary already generates the suggested values. Both `merge` and `run` call `runPipeline()`, so the flag can be threaded through once.

**Effort:** S
**Priority:** P2
**Depends on:** Phase 2 shipped (calibration data exists), Phase 3 shipped (run command exists)

### Stdout coupling in resolve.Run() and review.Run()

**What:** Refactor `resolve.Run()` and `review.Run()` to accept an `io.Writer` (or quiet flag) for progress output instead of hardcoded `fmt.Printf` calls.

**Why:** The `run` command calls both functions, and their stdout output interleaves with `run`'s own progress messages. Currently the messages happen to be sequential and informative, so it's acceptable. But as more commands compose these functions, stdout coupling becomes a problem.

**Context:** `resolve.Run()` prints "Merged N review clusters" and "Resolved N contacts". `review.Run()` prints "Reviewing N pending pairs...". These are fine standalone but the `run` command can't control the output. Low priority since the current output reads naturally.

**Effort:** S
**Priority:** P3
**Depends on:** Phase 3 shipped (run command exists)

## Completed

### Interactive CLI review command

**What:** `rolodex review --report report.json --review review.vcf` — BubbleTea TUI with adaptive pacing, undo stack, calibration logging, and end-of-session threshold suggestions.
**Completed:** v0.2.0.0 (2026-04-07)
