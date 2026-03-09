# Contributing to wnwn

Thanks for your interest in contributing.

## Development Setup

```bash
eval "$(mise activate bash)"
go test ./...
go build -o wnwn ./cmd/wnwn/
```

## Pull Request Expectations

- Keep changes focused and explain user impact in the PR description.
- Update docs (`README.md`, `STATUS.md`) when behavior changes.
- Ensure `go test ./...` passes locally before opening a PR.

## Commit Style

- Use concise, imperative subjects (for example `feat: ...`, `fix: ...`, `docs: ...`).
- Include context in commit body for non-obvious design decisions.

## Developer Certificate of Origin (DCO)

By contributing, you certify your work under the DCO 1.1.

Add a sign-off line to each commit:

```bash
git commit -s -m "feat: your change"
```

This adds:

`Signed-off-by: Your Name <you@example.com>`
