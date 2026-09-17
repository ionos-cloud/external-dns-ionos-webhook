# Agent notes for this repo

## CI/CD decisions — do not reconsider

The following were each measured and rejected during the ICNS-3663 CI/CD hardening pass on this repo.
Reopening any of them requires a new measurement, not an argument — file it against ICNS-2158 and update
"CI/CD decisions", which is the authority for these calls.

- Do not use any `paas-github-actions` asset or Vault here — structurally impossible from a public repo
  (that asset repo is private; this repo is public, so `uses:` cannot resolve it). There is also no Vault,
  PAT, or Harbor integration in this repo to migrate.
- Do not hand-roll `actions/cache` in place of `actions/setup-go` — measured at ~33 minutes/month saved at
  8.3 PR runs/month, below the bar for the added maintenance surface. Keep plain `setup-go`.
- Do not add Docker layer caching.
- Do not add `arm64` to the PR build path.
- Do not drop `windows`, `darwin`, or `386` from the release build matrix.
- Do not add `paths-ignore` to the required workflow's trigger before the required-check swap (registering
  the `Passed` aggregator context and removing the old `snapshot-release` context) is complete — doing so
  earlier risks silently skipping the still-required check on some PRs.
- Do not flip any `sha_pinning_required` flag before the SHA-pinning pass has actually landed across both
  workflow files.
