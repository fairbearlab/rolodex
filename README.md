# rolodex

A CLI tool that merges and deduplicates vCard (.vcf) files. Fix your contacts at the source instead of locking them into another app.

## Install

```
go install github.com/fairbearlab/rolodex/cmd/rolodex@latest
```

Or download a binary from [Releases](https://github.com/fairbearlab/rolodex/releases).

## Usage

### Merge

Export your contacts as .vcf files from iCloud and Google, then merge:

```
rolodex merge --icloud icloud.vcf --google google.vcf --out merged.vcf --report report.json
```

This produces:
- **merged.vcf** -- confidently merged contacts (auto-merge tier, score > 0.85)
- **review.vcf** -- uncertain matches that need your eyes (review tier, score 0.60-0.85)
- **report.json** -- every merge decision explained with confidence scores

### Review

Interactively review uncertain matches in your terminal:

```
rolodex review --report report.json --review review.vcf
```

The TUI walks through each review-tier pair one at a time. High-confidence pairs get a compact card, ambiguous pairs get a full field-by-field diff with score breakdown. Press `m` to merge, `s` to skip, `u` to undo, `d` to toggle detail level. Decisions are saved after every keypress, so you can quit with `q` and resume later.

At the end of a session, you get threshold suggestions based on your decisions, so future merges can auto-handle more pairs.

### Resolve

After reviewing (interactively or by editing report.json), apply decisions:

```
rolodex resolve --report report.json --review review.vcf --merged merged.vcf --out final.vcf
```

Import final.vcf back into iCloud or Google.

## How it works

1. **Parse** both .vcf files into a canonical contact model
2. **Normalize** phones (digits-only), emails (lowercase), names (Unicode NFKD, strip titles/suffixes)
3. **Block** candidates by shared email, phone, or last name to avoid N*M comparisons
4. **Score** each candidate pair: Jaro-Winkler name similarity (with nickname expansion like Bob/Robert), shared email, shared phone, shared org
5. **Merge** auto-merge tier contacts with iCloud priority for single-value fields, union for emails/phones
6. **Report** every decision with confidence scores and conflict details

## Scoring

| Signal | Weight |
|--------|--------|
| Name similarity (Jaro-Winkler) | 0.40 |
| Shared email | 0.25 |
| Shared phone | 0.25 |
| Shared org | 0.10 |

Tiers: **auto_merge** (> 0.85), **review** (0.60-0.85), **distinct** (< 0.60)

## License

MIT
