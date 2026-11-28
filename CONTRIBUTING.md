# Contributing to FAIR

FAIR exists to make distributed systems fairer and more predictable under resource constraints. We welcome contributions from the community and appreciate your time and effort in helping the project grow.

## How to Contribute

### Reporting Issues
- Use the Bug Report or Feature Request templates when opening issues.
- Be clear and specific: what’s the expected vs. actual behavior? Steps to reproduce?
- Include environment details (Go version, OS, runtime).

### Proposing Changes
- Before starting large changes, open an issue to discuss your idea.
- Fork the repo and create a feature branch (`feature/my-cool-change`).
- Write clear commit messages (present tense, short, descriptive).

### Submitting Pull Requests (PRs)
- Keep PRs focused — smaller PRs are easier to review and merge.
- Include tests where possible. If it’s hard to test, explain why.
- Update documentation and examples if your change affects usage.
- Be open to review feedback: the goal is clarity, correctness, and long-term maintainability.

## Review & Acceptance
All contributions are reviewed by project maintainers. Reviews consider:
- Correctness: Does the change solve the problem as intended?
- Efficiency: Is the implementation performant and resource-aware?
- Clarity: Is the code clean, well-structured, and readable?
- Tests: Are there adequate tests to cover edge cases?
- Docs: Is user-facing documentation updated where needed?

Feedback will be constructive and aimed at helping you get your contribution merged.

Maintainers make the final decision on acceptance, but the process is transparent — we’ll explain rejections and suggest paths forward.

## Code Style & Standards

Follow Go’s standard formatting (`go fmt`).
Keep functions and packages focused; avoid large, monolithic changes.

Write clear names for variables, functions, and tests.

## Community Values

- Be respectful and collaborative.
- No idea is too small — even typo fixes or doc improvements are valuable.
- FAIR is about fairness: we aim for fairness in both systems and our community.

## CI and release verification

The `Go` workflow's `verify` job runs on the latest Go release. It builds the checked-in source, runs the Go suite with coverage and with the race detector, runs the Python rotation conformance test, lints, scans for known vulnerabilities with `govulncheck`, and checks protobuf regeneration byte for byte. A second job, `oldest-supported-go`, builds and runs the race suite on the Go release that `go.mod` declares. The workflow also runs weekly, so a vulnerability published after the last push still fails a run; if the failure names the standard library, move the Go versions in the workflow forward. PRs (including stacked PRs) test GitHub's proposed merge commit. A push to `main` tests the actual merged commit; tag pushes test the tagged revision. CI reads repository contents only and does not update badges, create tags, or publish releases.

To reproduce the protobuf check from the repository root on Linux x86_64, run `bash .github/scripts/verify-proto.sh`. It uses temporary output, the SHA-256-pinned protoc 3.20.3 release (including its standard imports), and protoc-gen-go v1.36.10. For an intentional schema change, install those same generator versions, run `make proto`, and include the generated diff in the PR. When updating action pins, check the full commit SHA against the upstream release tag (peel annotated tags to the commit) and update the version comment too.

### Maintainer procedure: tag only a verified commit

1. Merge the reviewed PR to protected `main`. Wait for **the push run on `main`** to pass `verify`; a successful PR merge-ref run is not sufficient. Copy the full commit SHA from the successful push run.
2. Fetch `origin/main` and tags. Check `git merge-base --is-ancestor "$sha" origin/main` and confirm the intended version tag does not exist locally or on origin. Create the annotated tag at the explicit verified SHA, not at `HEAD` (for example, `git tag -a "$tag" "$sha" -m "$tag"`). Sign it if required by release policy.
3. Confirm `git rev-parse "$tag^{commit}"` equals that full SHA. Push only the new tag (`git push origin "refs/tags/$tag"`). Before publishing, confirm the remote tag resolves to the same commit and the tag's own `verify` run passed. Stop on mismatch or failure; never move or force-push a published tag.

### Hosted protections (maintainer configuration, not set by this workflow)

- On `main` and protected stack bases, require a reviewed PR, up-to-date status checks and the `verify` check from GitHub Actions. When migrating from older check names, select the new check after its first run. Block force pushes and deletions; restrict bypasses. Require review of workflow and verification-script changes.
- Set the default Actions token to read-only. Restrict release-tag creation to authorized maintainers and prevent tag updates/deletions with tag rulesets. Confirm these settings in the hosted repository: a workflow file cannot enforce branch or tag protection by itself.
