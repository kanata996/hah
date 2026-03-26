# Contributing to hah

Thanks for your interest in contributing.

## Development setup

1. Install Go (version from `go.mod`).
2. Clone the repo.
3. Run checks locally:

```bash
make ci
```

## Common commands

```bash
make test
make test-pkg PKG=./reqx
make test-name PKG=./reqx RUN=TestDecodeJSON
make test-cover
make test-race
```

## Pull requests

1. Keep PRs focused and small.
2. Add or update tests for behavior changes.
3. Ensure `make ci` passes before opening a PR.
4. Update docs when API behavior changes.

## Release flow

1. Merge the release PR into `main`.
2. Update your local release branch:

```bash
git checkout main
git pull --ff-only origin main
```

3. Create and publish the tag plus GitHub release:

```bash
make release VERSION=vX.Y.Z
```

If you release from a branch other than `main`, override `MAIN_BRANCH`:

```bash
make release VERSION=vX.Y.Z MAIN_BRANCH=release-branch
```

If the tag already exists on `origin` and you only need the GitHub release entry:

```bash
make release-gh VERSION=vX.Y.Z
```

## Code style

- Follow standard Go formatting (`gofmt`).
- Prefer clear naming and small, testable functions.
- Keep package boundaries strict (`hah`, `reqx`, `errx`, `resp`).
