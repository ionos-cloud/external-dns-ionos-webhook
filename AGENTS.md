# external-dns-ionos-webhook — agent notes

## CI/CD — decided, do not re-propose

Evaluated and rejected for this repo (2026-08). Every measurement below is reproducible from this
repo's own public Actions history. **Reopening one needs new data, not an argument.**

- **Hand-rolling `actions/cache` with `restore-keys` over `setup-go`.** `setup-go` restores on exact
  primary-key match only and its key hashes `go.sum`, so the ~53% of PR runs that are bot dependency
  bumps always cold-start (576s vs 122s warm). The absolute rate does not justify the maintenance
  though: 8.3 PR runs/month is ~33 min/month. Keep plain `setup-go`. The ~7.2 GiB cache footprint is a
  separate, reclaim-side problem — see "Wanted" below.
- **Docker layer caching.** The Dockerfile is a distroless base plus one `ADD` of a host-built binary.
  There are no layers to cache.
- **arm64 / multi-arch on the PR path.** The release already covers both arches and the whole build
  takes 19s. Do not regress the published two-arch manifest.
- **Dropping windows/darwin/386 from release artifacts.** Tempting — 212s of every cold run — but the
  download counts do not support it.
- **`paths-ignore` on the PR workflow.** Not until the required check has been swapped off
  `snapshot-release`; before that it silently un-gates `main`.
- **Flipping `sha_pinning_required` before the pinning pass.** It breaks every tag-ref workflow.
  After, not before.
- **`ionos-cloud/paas-github-actions` composites.** Not usable here: a public repo's workflow token
  cannot resolve `uses:` into a private repo. Public actions and local composites only.
- **Merge queue, test sharding.** Both evaluated. No working precedent to copy, and neither addresses
  a measured problem in this repo.

### Wanted — do not read the above as "no new tooling"

- `timeout-minutes` on every `runs-on` job, sized at ~2× measured p100. Never one blanket value, and
  note it is an invalid key on a job that calls a reusable workflow.
- **Trivy at `HIGH,CRITICAL`** — this repo has no vulnerability scanning of any kind today.
- **CodeQL default setup, secret scanning, push protection** — settings toggles, no workflow file,
  free on a public repo, all three currently off.
- **`zizmor`** alongside `actionlint`, non-blocking. It covers dangerous triggers, `uses:`
  SHA-pinning and over-broad `permissions:`, which `actionlint` does not.
- **`govulncheck`** non-blocking, after Trivy. Pin it via a go.mod `tool` directive, never `@latest`;
  and note it exits 0 regardless of findings under `-format json|sarif`, so parse the document.
- **Cache reclaim on PR close** — `refs/pull/*` entries are unreachable once a PR closes and still
  count against the 10 GB cap.

Tracking issue: ICNS-2158.
