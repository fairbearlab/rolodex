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

This merges your contacts, drops you into a terminal UI for uncertain matches, then resolves your decisions and writes a clean `final.vcf` once every pair has been decided. Quit with pending decisions and the workspace is preserved so you can resume instead of losing progress.

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

`merge` rejects any two of `--icloud`, `--google`, `--out`, `--review` and `--report` that point at the same file — case-insensitively, through symlinks, and including a `<path>.tmp` staging sibling — instead of silently overwriting one with the other. `--review` defaults to `review.vcf` next to `--out` rather than the current directory. Malformed entries in either input are skipped and reported on stderr, by both `merge` and `run`.

The **review** TUI walks through each uncertain pair one at a time. High-confidence pairs get a compact card, ambiguous pairs get a full field-by-field diff with score breakdown. Press `m` to merge, `s` to skip, `u` to undo, `d` to toggle detail level. Decisions are saved after every keypress.

At the end of a session, you get printed threshold suggestions based on your decisions (e.g. "lower auto_merge to 0.78"). They're informational only -- there's no flag to apply one; a maintainer would change the constant in source.

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

The first four sum to 1.0; birthday is a bonus and the total is capped at 1.0. Birthdays compare after normalization, so `1989-10-22`, `19891022`, `10/22/1989`, `October 22, 1989` and a no-year `--10-22` all agree. A value that cannot be read as a date is never evidence of a match. A shared phone or email only counts if it's plausible: a phone needs 7+ digits that aren't all the same digit, an email needs a local part and a dotted domain. Placeholders like `0`, `000-000-0000` and `unknown` don't confirm anything.

Tiers: **auto\_merge** (>= 0.85), **review** (0.60-0.85), **distinct** (< 0.60), with three rules layered on top of the score because real exports are sparse (most contacts carry a name and at most one identifier):

* An identical name **plus** a shared phone, email or birthday (a real one — a placeholder such as `000-000-0000`, `unknown` or a January 1st birthday is not evidence) is **auto\_merge**, even though the linear score for that shape is only 0.50-0.65 (0.65 with a shared phone or email, 0.50 with only a shared birthday). "Identical" means the given and family names are equal (or one is a nickname of the other — Chris/Christopher, but not Eric/Erica and not two different diminutives like Ted/Ned), there is a family name and the given name is more than an initial (`Alex` / `Alex` or `J. Smith` / `J. Smith` are not identity — `J.R.` and `J R` count as initials too), middle names are compatible (a missing or matching initial is fine, including one Google folded into the given name), and generational suffixes agree (John Smith Jr. is not John Smith Sr., or John Smith). Names must also agree once accents are kept: `Nguyên` and `Nguyễn`, or `René` and `Rene`, go to review instead of auto-merging on a shared phone alone. Compatibility variants still count as one name — halfwidth kana and fullwidth Latin match as before.
* A near-identical name (similarity >= 0.95) on its own is **at least review**. Same-name pairs are never silently marked distinct; a shared org alone does not auto-merge them, because two people can share a common name and an employer. They are reviewed as **pairs**: same-name contacts are never chained into one cluster, so a common name in both exports does not become a single merge-all card.
* Two well-formed birthdays that disagree **cap the pair at review**, whatever else matches. Same name and a shared household phone is exactly how a parent and child look; the birthdays are what tell them apart. If one birthday cannot be read, the pair is held at review rather than merged on a single identifier.

When either contact lacks a given name, weights shift to 0.45 email / 0.45 phone / 0.10 org / 0.10 birthday, and at least two matching identifiers are required for auto-merge.

## Merge behavior

When contacts merge, single-value fields use **iCloud priority** -- the iCloud value wins for name, title, org, birthday, note, URL, and photo. If the iCloud contact is missing a field, it falls through to the Google value.

Multi-value fields (emails, phones, addresses) are **unioned** -- duplicates are removed by normalized value, and both sources' unique entries are kept.

All other vCard properties not explicitly modeled (e.g. `X-*` extension fields, `CATEGORIES`, `NICKNAME`) are preserved via a catch-all passthrough (values only -- parameters on an unmodeled field, and any control characters in it, are not preserved).

Output vCards include provenance extension fields: `X-ROLODEX-SOURCE` (which sources contributed), `X-ROLODEX-SCORE` (confidence score), and `X-ROLODEX-REVIEW` (whether the contact was flagged for review).

## Supported fields

Rolodex parses and merges these vCard 3.0 properties:

N, FN, EMAIL, TEL, ORG, TITLE, BDAY, ADR, NOTE, URL, PHOTO

All other properties present in the input are carried through to the output, with control and bidi-override characters stripped from every field at parse time.

## License

MIT
