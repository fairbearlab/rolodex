# rolodex

A CLI tool that merges and deduplicates vCard (.vcf) files. Fix your contacts at the source instead of locking them into another app.

## Install

```Shell
go install github.com/fairbearlab/rolodex/cmd/rolodex@latest
```

Or download a binary from [Releases](https://github.com/fairbearlab/rolodex/releases).

## Usage

### Merge

Export your contacts as vCard 3.0 (.vcf) files from iCloud and Google, then merge. Both services export 3.0 by default.

```Shell
rolodex merge --icloud icloud.vcf --google google.vcf --out merged.vcf --report report.json
```

This produces:

* **merged.vcf** -- confidently merged contacts (auto-merge tier, score > 0.85)
* **review\.vcf** -- uncertain matches that need your eyes (review tier, score 0.60-0.85)
* **report.json** -- every merge decision explained with confidence scores

### Review

Interactively review uncertain matches in your terminal:

```Shell
rolodex review --report report.json --review review.vcf [--calibration cal.jsonl]
```

The TUI walks through each review-tier pair one at a time. High-confidence pairs get a compact card, ambiguous pairs get a full field-by-field diff with score breakdown. Press `m` to merge, `s` to skip, `u` to undo, `d` to toggle detail level. Decisions are saved after every keypress, so you can quit with `q` and resume later.

At the end of a session, you get threshold suggestions based on your decisions, so future merges can auto-handle more pairs. Decisions are also logged to a calibration JSONL file (default: alongside report.json) for analysis.

### Resolve

After reviewing (interactively or by editing report.json), apply decisions:

```Shell
rolodex resolve --report report.json --review review.vcf --merged merged.vcf --out final.vcf
```

Import final.vcf back into iCloud or Google.

### Version

```Shell
rolodex version
```

## How it works

1. **Parse** both .vcf files into a canonical contact model
2. **Normalize** phones (digits-only), emails (lowercase), names (Unicode NFKD, strip titles/suffixes)
3. **Block** candidates by shared email, phone, or last name to avoid N\*M comparisons
4. **Score** each candidate pair: Jaro-Winkler name similarity (with nickname expansion like Bob/Robert), shared email, shared phone, shared org
5. **Merge** auto-merge tier contacts via union-find clustering with iCloud priority
6. **Write** merged.vcf (confident merges + singletons) and review\.vcf (uncertain clusters)
7. **Report** every decision to report.json with confidence scores and conflict details
8. **Review** (optional) walk through uncertain pairs in a terminal TUI, then **resolve** to apply decisions into a final.vcf

## Scoring

| Signal                         | Weight |
| ------------------------------ | ------ |
| Name similarity (Jaro-Winkler) | 0.40   |
| Shared email                   | 0.25   |
| Shared phone                   | 0.25   |
| Shared org                     | 0.10   |

Tiers: **auto\_merge** (> 0.85), **review** (0.60-0.85), **distinct** (< 0.60)

When either contact lacks a given name, weights shift to 0.45 email / 0.45 phone / 0.10 org, and at least two matching identifiers are required for auto-merge.

## Merge behavior

When contacts merge, single-value fields use **iCloud priority** -- the iCloud value wins for name, title, org, birthday, note, URL, and photo. If the iCloud contact is missing a field, it falls through to the Google value.

Multi-value fields (emails, phones, addresses) are **unioned** -- duplicates are removed by normalized value, and both sources' unique entries are kept.

All other vCard properties not explicitly modeled (e.g. `X-*` extension fields, `CATEGORIES`, `NICKNAME`) are preserved via a catch-all passthrough.

Output vCards include provenance extension fields: `X-ROLODEX-SOURCE` (which sources contributed), `X-ROLODEX-SCORE` (confidence score), and `X-ROLODEX-REVIEW` (whether the contact was flagged for review).

## Supported fields

Rolodex parses and merges these vCard 3.0 properties:

N, FN, EMAIL, TEL, ORG, TITLE, BDAY, ADR, NOTE, URL, PHOTO

All other properties present in the input are carried through to the output unchanged.

## License

MIT
