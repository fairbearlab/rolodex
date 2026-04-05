# TODOS

## Merge Engine

### Calibration dataset for scoring thresholds

**What:** Create a labeled set of 50-100 contact pairs from real exports, marked as match/non-match. Validate the auto_merge (0.85) and review (0.60) thresholds against real data.

**Why:** Thresholds are reasonable starting points from entity resolution prior art, but every contact dataset is different. Real validation is the only way to know if the scoring model actually prevents silent wrong merges.

**Context:** The design doc's success criteria already calls for this ("verified by manually labeling 50-100 contact pairs"). Requires real contact data (PII), so the labeled set should be anonymized or kept in a private, gitignored directory. Run after scorer is implemented. Compare precision at auto_merge threshold, recall at review threshold.

**Effort:** M
**Priority:** P1
**Depends on:** Scorer implementation

### Interactive CLI resolve command

**What:** `rolodex review --report report.json` walks through each review-tier pair interactively, showing contacts side by side with merge/skip prompts.

**Why:** The report-driven resolve (edit JSON, run resolve) works but requires hand-editing JSON. An interactive walkthrough is much better UX for reviewing 10-50 flagged pairs.

**Context:** Phase 1 ships the report-driven resolve command. Both workflows write decisions to the same report.json format, so the outputs are identical. The interactive command is a UX layer on top of the same resolve logic. May require a TUI library or at minimum formatted terminal output + stdin prompts.

**Effort:** M
**Priority:** P2
**Depends on:** resolve command (Phase 1)

### Per-field provenance tracking

**What:** For merged contacts, track which source each field came from. Example: in report.json, `"name": {"value": "Bob Smith", "source": "icloud"}, "emails": [{"value": "bob@gmail.com", "source": "google"}]`.

**Why:** Contact-level provenance (X-ROLODEX-SOURCE:merged(icloud+google)) tells you the contact was merged but not which source won each field. Per-field provenance enables future features like rollback and makes the report much richer for manual review.

**Context:** Phase 1 has contact-level provenance + per-merge explanations in report.json. Per-field provenance is the natural next step. Adds complexity to the merge and report stages. The report.json already captures some of this in the decision explanations, so there's partial overlap to reconcile.

**Effort:** M
**Priority:** P3
**Depends on:** Merge stage must be stable first

## Completed
