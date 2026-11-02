# risk-zones.md: rolodex

> DRAFT (agent-drafted 2026-10-13 from git history at origin/main@8581a58; user-reviewed 2026-10-19; zones recalibrated 2026-10-30 (red = release, contact-loss writes, privacy and agent guardrails); the user edits, agents never do). Read by L0 triage, the /ship prefix, A6 scope judge, A7 evidence check and L7. Schema version 1.

## How to use
1. List the changed paths (`git diff --name-only <base>...`).
2. Each path takes the HIGHEST zone of every row whose glob matches it. A path no row matches is **yellow** (round up). A path is prod-touching if ANY matching row says Prod yes; its evidence classes are all classes named by the matching rows. Prod is yes only when the change itself acts on prod (deploy or release paths, live data or DB writes and migrations, prod secrets, prod config the running system reads directly, or files that are live when edited); code that reaches prod later through the normal reviewed path is no, and its Zone carries the risk.
3. The change's zone is the highest zone of any changed path. One-way changes (data migration, deleted history, published release, public post) are red whatever this file says.
4. Prefix /ship with the row from "Ship prefix" below. If any changed path is prod-touching, add L7.

## Zones
| Glob | Zone | Prod | Evidence class | Why (evidence) |
|---|---|---|---|---|
| `.github/workflows/release.yml`, `.goreleaser.yaml` | red | yes | orchestration | Release: a `v*` tag (pushed only by the user, user-confirmed) publishes the binaries. |
| `.github/workflows/**` | yellow | no | orchestration | Merge gate and security scans; review any dropped job, required check or floor; actions SHA-pinned. |
| `.github/dependabot.yml`, `Makefile`, `.golangci.yml` | yellow | no | config | Bump config, COVER_MIN floor and lint policy; a lowered floor or new exclude needs a callout in review. |
| `**/.claude/**`, `**/CLAUDE.md`, `**/*.local.md`, `.githooks/**`, `.pre-commit-config.yaml`, `risk-zones.md`, `.loops/**`, `bin/env-check` | red | no | none | Always red: agent and hook config (gitignored here), the env-check harness, and this triage policy. |
| `.gitignore` | red | no | config | Always red: the only barrier keeping real contacts and private notes out of a public repo; editing it publishes nothing by itself. |
| `**/*.vcf`, `**/*.jsonl`, `**/report.json`, `notes/**`, `**/.env*`, `**/*secret*`, `**/*credential*` | red | yes | config | Always red: committing a real export or private note publishes it (one-way). |
| `VERSION`, `cmd/rolodex/version.txt`, `go.mod`, `go.sum` | yellow | no | config | Version and deps of the next binary; `make version-check` guards drift; escalate a major or parser-path dep bump. |
| `cmd/rolodex/*.go` | red | no | orchestration | Destructive writes on the user's real exports (O_TRUNC, RemoveAll) and the symlink-overwrite guard. |
| `internal/{writer,resolve,merger,scorer,parser,normalize,model}/**` | red | no | none | Contact-loss path: the .vcf writer, auto-merge decisions and thresholds, the untrusted-input parser. |
| `internal/{blocker,prune,reporter,calibration}/**` | yellow | no | none | Shape which contacts get reviewed or pruned; blocker has the open OOM P0. |
| `internal/review/**` | red | no | ui | TUI keys write the decisions that resolve deletes contacts by; weakest-tested package. |
| `testdata/**` | yellow | no | none | Synthetic fixtures; `.vcf` stays red via the privacy row on purpose. |
| `*.md`, `LICENSE` | green | no | none | Docs and license; agent config rows above win. |

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
