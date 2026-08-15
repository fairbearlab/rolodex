# Security Policy

## Supported versions

Only the latest release of rolodex receives security fixes.

## Reporting a vulnerability

Please **do not** open a public issue for security problems. Use GitHub's private
vulnerability reporting: **Security → Report a vulnerability** on this repository.
You'll get an acknowledgement within 7 days.

## What's already in place

- Dependencies are monitored by Dependabot; CI runs a vulnerability scan on every push.
- GitHub Actions are pinned to commit SHAs and run with minimal token permissions.
- This repository is scored by [OpenSSF Scorecard](https://scorecard.dev/viewer/?uri=github.com/fairbearlab/rolodex).
