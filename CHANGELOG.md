# Changelog

## [0.1.0.0] - 2026-04-04

First working release. Merge and deduplicate vCard files from iCloud and Google exports with confidence scoring and explainable decisions.

### Added

- **vCard parser** reads iCloud and Google .vcf exports into a canonical contact model. Handles structured names, multi-value emails/phones, addresses, photos, and extra fields. Malformed entries produce warnings instead of aborting.
- **Contact normalizer** with Unicode NFKD + accent stripping, case folding, title/suffix removal for names. Digits-only phone normalization with US country code stripping.
- **Candidate blocker** groups contacts by shared email, phone, or last name to avoid N*M comparisons. Large last-name blocks apply secondary filter.
- **Similarity scorer** with Jaro-Winkler name comparison and ~120 nickname mappings (Bob/Robert, Bill/William, etc.). Weighted scoring: name 0.40, email 0.25, phone 0.25, org 0.10.
- **Merger** with union-find clustering and pairwise validation. iCloud priority for single-value fields, union for emails/phones. Passthrough for fields present in only one source.
- **vCard 3.0 writer** with X-ROLODEX-SOURCE, X-ROLODEX-SCORE, and X-ROLODEX-REVIEW provenance extension fields.
- **JSON reporter** with summary stats, per-merge decisions with conflict details, review-tier ambiguities, and distinct entries.
- **Resolve command** applies user decisions from an edited report.json to review-tier contacts.
- **CLI** with `merge` and `resolve` subcommands.
- **CI/CD** with GitHub Actions for test-on-push and GoReleaser for cross-platform binary releases.
- **30+ tests** including unit tests, integration test, benchmark (1000 contacts in ~26ms), and fuzz targets.
