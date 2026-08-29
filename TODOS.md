# TODOS

## Merge Safety

### Cap the email and phone blocking buckets

**What:** `internal/blocker/blocker.go` caps the last-name bucket at `maxLastNameBlockSize = 50` and falls back to `addFilteredPairs`, but the email and phone buckets have no cap. Apply the same cap-and-filter path to both, and warn when a bucket is truncated. Independently, cap cluster size in `merger.Merge` (or refuse to auto-apply a `merge` decision above N members).

**Why:** One shared identifier produces an O(k^2) pair explosion **and** a single unbounded review cluster. Measured on a real build: 500 contacts sharing one `TEL` collapse into ONE review cluster of 501 members — the TUI shows it as a single card and `resolve.mergeReviewCluster` fuses all 501 into one contact on a single `m` keystroke. 4,000 such contacts produce 7,998,000 candidate pairs, 9.1s and 3.99 GB RSS; ~8,000 is an OOM kill. This is the same hazard `isNearNameOnly` was written for, on the path that guard does not cover.

**Context:** Found independently by the security specialist and the adversarial pass during the v0.4.0 pre-landing review. Entirely reachable without malice: a company switchboard, a family landline, or a placeholder like `000-000-0000` that some exports emit. `.vcf` input is untrusted, so it is also a trivial DoS. This is the single worst remaining bug in the tool.

**Effort:** M
**Priority:** P0
**Depends on:** Nothing

### `[s] skip` destroys both contacts with no warning

**What:** `internal/resolve/resolve.go` excludes a skipped cluster from `output` entirely. Review-cluster members are not in `merged.vcf` (`merger.go` routes them exclusively to `result.Review`), so `skip` is the only decision that deletes data — `pending` and `q` both keep everything. Either rename the key to `[s] discard both` with a confirmation, or change the semantics to emit both contacts separately.

**Why:** On a card asking "are these the same person?", `[s] skip` reads as "no" or "not now" — which is exactly when a reviewer wants **both** kept. A reviewer working 200 near-name pairs and pressing `s` on the genuinely-different ones deletes 400 people from their address book.

**Context:** Documented in ARCHITECTURE.md and asserted in `resolve_test.go`, so the behaviour is intentional; the problem is that the label does not match it. Needs a product decision, not a bug fix.

**Effort:** S
**Priority:** P1
**Depends on:** Nothing

### Bind cluster ids to reviewed contact content

**What:** `merger.ClusterID` hashes `source:index:family:given` per member. Add the decision-relevant field values (or a per-run identifier) to the hash, so a `review.vcf` from a different run cannot satisfy the id check.

**Why:** `BuildClusters` now rejects a `review.vcf` whose `X-ROLODEX-CLUSTER` tags disagree with `report.json`, which catches reordering and staleness. It does not catch a file from a different run with the same contact order and names but changed phones, emails or birthdays: the ids match, the decision is recorded against the new report, and `resolve` emits the stale contact data.

**Context:** Found by the Codex structured review during the v0.4.0 pre-landing pass. Changing the hash input breaks resume compatibility again, so it is worth batching with any other change that does.

**Effort:** S
**Priority:** P2
**Depends on:** Nothing

### Same-source field conflicts are dropped and unreportable

**What:** `merger.mergeCluster` fills a single-value field only when the base's is empty, so on a 3-member cluster the second same-source contact's `NOTE`, `ORG`, `TITLE`, `BDAY`, `URL` and `PHOTO` are discarded. `reporter.findConflicts` compares only the first iCloud contact against the first non-iCloud one, so it structurally cannot report a same-source conflict. Compare every member pairwise, and write a report by default.

**Why:** Two iCloud "John Smith" cards with `NOTE:Met at conf` and `NOTE:Owes me $500`, plus a Google card sharing a phone: one note survives, silently. `merge --report` defaults to `""` and `run` deletes the temp report on success unless `--report` is passed, so in the default flows the loss is recorded nowhere.

**Effort:** M
**Priority:** P2
**Depends on:** Nothing

### Validate vCard TYPE parameters

**What:** `writer.contactToCard` copies attacker-controlled `TYPE` parameter values straight into `vcard.Params`, and go-vcard's encoder escapes only backslash, LF and comma — never `;` or `:`. Validate against the known type tokens in `parser.fieldType`, or reject any parsed param value containing `;`, `:`, `"`, CR or LF. Same for `PhotoType`.

**Why:** `EMAIL;TYPE="X:evil@attacker.test,":real@good.test` is written back as `EMAIL;TYPE=X:EVIL@ATTACKER.TEST:real@good.test`, which readers parse as the address `EVIL@ATTACKER.TEST:real@good.test` — the genuine address is corrupted in the user's merged export.

**Context:** Cannot forge a whole new property (LF is escaped). Control characters are now stripped at the parse boundary, which closed the related CR-injection path; this is the remaining `;`/`:` case.

**Effort:** S
**Priority:** P2
**Depends on:** Nothing

### Preserve vCard property groups (Apple `item1.` labels)

**What:** go-vcard strips the group prefix (`item1.EMAIL` / `item1.X-ABLabel:School`) before the parser sees a field, and the model has no place for it, so every written card comes out with bare `EMAIL` and a detached `X-ABLABEL` that labels nothing. Capture `vcard.Field.Group` on the modeled multi-value fields (email, phone, address, URL) and on `Extra`, and re-emit the prefix in the writer.

**Why:** Apple exports carry every custom label this way. After `merge`, `resolve` or `prune` the labels are separated from the values they named, silently. `prune` is advertised as a faithful split, and this is the largest remaining thing it loses.

**Context:** Found by the adversarial review of v0.5.0. Documented as a limitation in README (Merge behavior) until fixed. A second `URL` or `NOTE` on one card is dropped for the same single-value-field reason and belongs to the same fix.

**Effort:** M
**Priority:** P1
**Depends on:** Nothing

### `resolve` and the review loader discard parse warnings

**What:** `internal/resolve/resolve.go` (`parser.ParseFile(mergedPath, "merged")`) and `internal/resolve/loader.go` (`review.vcf`) drop the warnings slice. A malformed `merged.vcf` loses contacts from `final.vcf` with no message, the same silent loss `merge`, `run` and `prune` now report. Surface them through `reportParseWarnings` (or refuse, as `prune --out` does).

**Why:** A truncated intermediate file is the one case the cluster-id check cannot catch, because the lost card is simply absent.

**Effort:** S
**Priority:** P2
**Depends on:** Nothing

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

### Hoist per-contact work out of the per-pair scoring loop

**What:** Cache each contact's parsed birthday and split given/middle tokens on `NormalizedContact` during `normalize.Contact`, and have `sharedBirthday`, `birthdayConflict`, `birthdayUnknown`, `sameName` and `sameGivenName` read the cached values. `scorePair` currently runs eight `parseBirthday` calls per pair and re-tokenizes both given names on every comparison.

**Why:** All of this work depends only on each contact individually, never on the pairing, so it repeats identically every time either contact appears in a candidate pair.

**Context:** Not urgent at current scale — `blocker.Block` prunes to shared email, phone or last-name buckets, so the author's real export produces 559 candidate pairs, where the redundant work costs microseconds. It becomes real if the blocking buckets are ever widened, and it compounds with the uncapped-bucket bug above.

**Effort:** M
**Priority:** P3
**Depends on:** Cap the email and phone blocking buckets

### Decide whether an unknown birthday should cap the score path too

**What:** `scorer.Classify` has `nameRule := f.NameExact && confirmed && !f.BirthdayUnknown`, but the tier switch is `case autoMerge && !f.BirthdayConflict`. A `BirthdayConflict` caps both paths; a `BirthdayUnknown` caps only the single-identifier name rule. Decide whether to add `&& !f.BirthdayUnknown` to the switch.

**Why:** A pair with a shared email, phone and org reaches 0.95 and auto-merges even though one birthday is unreadable and might be a conflict. The adversarial pass called this the same asymmetry class the repo has been bitten by twice.

**Context:** Deliberate as it stands — `TestBirthdayGuardFailsClosed` asserts it with the comment "the unknown birthday only withholds the single-identifier shortcut", and two shared identifiers is not "name alone". Tightening it would grow the review queue. This is a threshold-calibration call, best made against the labelled dataset below.

**Effort:** S
**Priority:** P3
**Depends on:** Calibration dataset for scoring thresholds

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

### `run` exits 0 when the review is paused

**What:** `cmd/rolodex/main.go` returns without `os.Exit(1)` on `ErrReviewPaused`. Exit non-zero (or document the code) so a wrapping script can tell a paused review from a completed one.

**Why:** No `final.vcf` was written, but a Makefile or CI step sees success. The guidance is printed to stderr, so an interactive user is informed; an automated one is not.

**Effort:** S
**Priority:** P2
**Depends on:** Nothing

### Warn about PII left in the temp workspace

**What:** On a pipeline error or a paused review, `rolodex run` preserves the temp workspace and nothing ever removes it. Say plainly that it holds the user's full contact data, and/or place it under `os.UserCacheDir` and prune old workspaces on the next run.

**Why:** The directory holds `merged.vcf`, `review.vcf` and `report.json` — every name, phone, email, birthday and address, plus conflict entries recording both sources' values. Permissions are correct (0700/0600), so this is retention rather than access control, but a full copy accumulates after every failed run with only a stderr line as notice.

**Context:** `internal/calibration` is clean by contrast — cluster hash, score and feature booleans only, opened 0600.

**Effort:** S
**Priority:** P3
**Depends on:** Nothing

## Completed

### Interactive CLI review command

**What:** `rolodex review --report report.json --review review.vcf` — BubbleTea TUI with adaptive pacing, undo stack, calibration logging, and end-of-session threshold suggestions.
**Completed:** v0.2.0 (2026-04-07)
