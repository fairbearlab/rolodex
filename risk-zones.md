# risk-zones.md: rolodex

> DRAFT (agent-drafted 2026-10-13 from git history at origin/main@8581a58; user-reviewed 2026-10-19; the user edits, agents never do). Read by L0 triage, the /ship prefix, A6 scope judge, A7 evidence check and L7. Schema version 1.

## How to use
1. List the changed paths (`git diff --name-only <base>...`).
2. Each path takes the HIGHEST zone of every row whose glob matches it. A path no row matches is **yellow** (round up). A path is prod-touching if ANY matching row says Prod yes (treat ? as yes); its evidence classes are all classes named by the matching rows.
3. The change's zone is the highest zone of any changed path. One-way changes (data migration, deleted history, published release, public post) are red whatever this file says.
4. Prefix /ship with the row from "Ship prefix" below. If any changed path is prod-touching, add L7.

## Zones
| Glob | Zone | Prod | Evidence class | Why (evidence) |
|---|---|---|---|---|
| `.github/workflows/**` | red | yes | orchestration | Always red. release.yml publishes the GitHub Release binaries on a `v*` tag push (goreleaser, `contents: write`; 3 commits: created in 208575a, then 57fd373 and 48a34d4). ci.yml is the merge gate: shared go-ci.yml pinned @5a38bfa7 (lint, go vet, race tests, go mod tidy -diff, govulncheck; verified), coverage floor and bench smoke (5 commits). The main ruleset (no bypass actors) blocks deletion and force-push, requires a PR and strict status checks go / lint, go / test, go / vulncheck, bench, analyze (go). No tag ruleset: only the user pushes a `v*` tag, never an agent (user-confirmed). codeql.yml and scorecard.yml hold security-events / id-token write. All actions SHA-pinned. |
| `.github/dependabot.yml`, `Makefile`, `.golangci.yml` | red | no | config | CI gate config. dependabot.yml steers weekly gomod and github-actions bump PRs (the SHA pins); rounded up. Makefile holds the CI gate logic: COVER_MIN=60 floor, version-check, `make ci`; golangci enables gosec with a no-excludes / no-nolint policy. Lowering either silently weakens CI. |
| `**/.claude/**`, `**/CLAUDE.md`, `**/*.local.md`, `.githooks/**`, `.pre-commit-config.yaml` (anywhere for the first three, since .gitignore matches them at any depth; all absent from the tree today) | red | no | none | Always red. Agent config and hook scripts; CLAUDE.md and .claude/ are gitignored, so any commit of them is red. |
| `.gitignore`, `**/*.vcf`, `**/*.jsonl`, `**/report.json` (anywhere; .gitignore only covers the root; none tracked), `notes/**`, `**/.env*`, `**/*secret*`, `**/*credential*` (defensive) | red | yes | config | Secrets and privacy guard for a PUBLIC repo: .gitignore is the only barrier keeping real contact exports, calibration.jsonl, private local notes (notes/) and agent config out of git (5 commits). Committing a .vcf/.jsonl/report.json anywhere publishes real contacts (one-way). The binary reads no env vars and holds no secrets; release.yml uses only GITHUB_TOKEN. Always red. |
| `.goreleaser.yaml`, `VERSION`, `cmd/rolodex/version.txt`, `go.mod`, `go.sum` | red | yes | config | Release surface: published artifacts (3 OS x 2 arch, checksums), the version users see (VERSION and the go:embed-ed version.txt must agree, enforced by `make version-check`), and the dependency set and toolchain (go 1.26.1) of the shipped binary; vCard parsing deps sit on the untrusted-input path. They drifted twice before a tag (0.1.0.0 vs 0.3.0.0, synced in 7c7da4f; again during the v0.4.0 work); d241719 added `make version-check` to `make ci`. No tagged release shipped a wrong version. `go install ...@latest` is a documented install path (README). No known users besides the user (user-confirmed). VERSION 6 commits, version.txt 5; .goreleaser.yaml unchanged since 208575a. Weekly gomod Dependabot; one bump on main (8581a58). |
| `cmd/rolodex/**` | red | yes | orchestration | Destructive file ops on the user's real exports: O_TRUNC, RemoveAll, Remove of stale review / calibration files; main.go's checkDistinctPaths guard (a prior bug overwrote a source export through a symlinked dir, d241719). Includes cmd/rolodex's 7 test files (path_guard_test.go guards that symlink-overwrite bug) and any new subcommand file. pipeline.go 6 commits; prune.go is v0.5.0 (2105b51), least field time. |
| `internal/{writer,resolve,merger,scorer,parser,normalize,model}/**` | red | yes | none | Contact-loss path. writer is the only .vcf writer (temp, fsync, rename over the target; 5 commits). resolve and merger produce final.vcf and auto-merges (v0.4.0 "silent contact-loss paths"; TODOS P1 `[s] skip` drops both contacts). scorer decides auto_merge without review (irreversible once written). parser is the single untrusted-input ingress (fuzz + escape tests). model holds ThresholdAutoMerge (0.85), a flip for auto-merge without review; normalize decides the plausibility and birthday gates scorer uses. model/contact.go has 6 commits and only 1.6 KB of tests for 8.9 KB of model source (contact.go 6.6 KB + report.go 2.3 KB). All rounded up for irreversibility. |
| `internal/{blocker,prune,reporter,calibration}/**` | yellow | yes | none | Ship in the binary and shape which contacts land in removed.vcf or get reviewed. blocker has the open TODOS P0 (uncapped email/phone buckets, OOM); prune is 6 weeks old. The open P0 fix (fix/cap-blocking-buckets, PR #22, planned to land with PR #20 fix/merge-safety) also edits merger.go and pipeline.go, so that change is red as a whole. |
| `internal/review/**` | red | yes | ui | Bubble Tea TUI whose merge/skip keys write the decisions in report.json; resolve deletes both contacts of a skipped cluster (TODOS P1) and fuses a whole cluster on one `m` (TODOS P0). Weakest-tested package (27.4% when the 60% floor was set); view.go 446 lines changed (+349/-97) in v0.4.0 (d241719), 3 commits. Needs terminal verification. |
| `testdata/**` | yellow | no | none | Two 5-card fixtures (google.vcf, icloud.vcf), unchanged since 208575a. Both fixtures are synthetic (555 numbers, generic names, example/acme/startup domains; checked 2026-10-19). The .vcf files stay red via `**/*.vcf` on purpose: a new fixture with a real contact is a public post. |
| `*.md`, `LICENSE` | green | no | none | README, ARCHITECTURE, CHANGELOG, TODOS, SECURITY, MIT license; not executed (goreleaser builds its changelog from git). Agent config rows above win for CLAUDE.md. |

Note: `{a,b}` expands as in bash; `*` does not cross `/`. Test files take the zone of the package row they sit under (there is no separate test row). Left to the yellow default on purpose: `.tool-versions` (local Go and golangci-lint pins; CI takes Go from go.mod). No server, database, auth, payments, migrations or network code exist; "prod" here means the published release binaries, `go install ...@latest`, and the user's real .vcf exports the tool reads and overwrites.

## Ship prefix
- red: "/ship — red zone (<category>): full review + <one extra review chosen at L0>"
- yellow: "/ship — yellow zone: routine review, red-team focus on <area>"
- green: "/ship" (routine)
- docs-only: "/ship — docs only: minimal review"
- trivial S0: "Minimal ship. Skip the in-depth review."

## CLAUDE.md line (to add to the repo)
"Before /ship, look up changed paths in risk-zones.md and prefix the invocation with the row."

## Open questions for the user
None open. All 6 answered or settled from the repo on 2026-10-19.
