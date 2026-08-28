# rolodex

[![CI](https://github.com/fairbearlab/rolodex/actions/workflows/ci.yml/badge.svg)](https://github.com/fairbearlab/rolodex/actions/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/fairbearlab/rolodex/badge)](https://scorecard.dev/viewer/?uri=github.com/fairbearlab/rolodex)
[![codecov](https://codecov.io/gh/fairbearlab/rolodex/graph/badge.svg)](https://codecov.io/gh/fairbearlab/rolodex)
[![Release](https://img.shields.io/github/v/release/fairbearlab/rolodex?sort=semver)](https://github.com/fairbearlab/rolodex/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> Your contacts contain four copies of your mom. This fixes that.

A CLI tool that merges and deduplicates vCard (.vcf) files. Fix your contacts at the source instead of locking them into another app.

## Install

```Shell
go install github.com/fairbearlab/rolodex/cmd/rolodex@latest
```

Or download a binary from [Releases](https://github.com/fairbearlab/rolodex/releases).

## Usage

### Quick start

Export your contacts as vCard 3.0 (.vcf) files from iCloud and Google, then run:

```Shell
rolodex run --icloud icloud.vcf --google google.vcf
```

This merges your contacts, drops you into a terminal UI for uncertain matches, resolves everything on exit, and writes a clean `final.vcf`. One command, done.

Use `--report report.json` to save the full merge report, or `--keep` to preserve intermediate files alongside the output.

### Audit

Find contacts you can't actually reach (no email and no phone):

```Shell
rolodex audit contacts.vcf
rolodex audit contacts.vcf --format json
```

Works on any VCF file, not just rolodex output.

### Individual commands

The `run` command wraps these three steps. You can also run them separately for more control:

```Shell
# Step 1: Merge
rolodex merge --icloud icloud.vcf --google google.vcf --out merged.vcf --report report.json

# Step 2: Review uncertain matches interactively
rolodex review --report report.json --review review.vcf

# Step 3: Apply decisions
rolodex resolve --report report.json --review review.vcf --merged merged.vcf --out final.vcf
```

The **review** TUI walks through each uncertain pair one at a time. High-confidence pairs get a compact card, ambiguous pairs get a full field-by-field diff with score breakdown. Press `m` to merge, `s` to skip, `u` to undo, `d` to toggle detail level. Decisions are saved after every keypress.

At the end of a session, you get threshold suggestions based on your decisions, so future merges can auto-handle more pairs.

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
| Shared birthday                | 0.10   |

The first four sum to 1.0; birthday is a bonus and the total is capped at 1.0. Birthdays compare after normalization, so `1989-10-22`, `19891022`, `10/22/1989`, `October 22, 1989` and a no-year `--10-22` all agree. A value that cannot be read as a date is never evidence of a match.

Tiers: **auto\_merge** (>= 0.85), **review** (0.60-0.85), **distinct** (< 0.60), with three rules layered on top of the score because real exports are sparse (most contacts carry a name and at most one identifier):

* An identical name **plus** a shared phone, email or birthday is **auto\_merge**, even though the linear score for that shape is only 0.65. "Identical" means the given and family names are equal (or one is a nickname of the other — Chris/Christopher, but not Eric/Erica and not two different diminutives like Ted/Ned), there is a family name and the given name is more than an initial (`Alex` / `Alex` or `J. Smith` / `J. Smith` are not identity), middle names are compatible (a missing or matching initial is fine, including one Google folded into the given name), and generational suffixes agree (John Smith Jr. is not John Smith Sr., or John Smith).
* A near-identical name (similarity >= 0.95) on its own is **at least review**. Same-name pairs are never silently marked distinct; a shared org alone does not auto-merge them, because two people can share a common name and an employer. They are reviewed as **pairs**: same-name contacts are never chained into one cluster, so a common name in both exports does not become a single merge-all card.
* Two well-formed birthdays that disagree **cap the pair at review**, whatever else matches. Same name and a shared household phone is exactly how a parent and child look; the birthdays are what tell them apart. If one birthday cannot be read, the pair is held at review rather than merged on a single identifier.

When either contact lacks a given name, weights shift to 0.45 email / 0.45 phone / 0.10 org / 0.10 birthday, and at least two matching identifiers are required for auto-merge.

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
