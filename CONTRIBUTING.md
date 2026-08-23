# Contributing to Spool

Thanks for contributing to Spool. This guide covers the local development workflow and the
repository conventions that keep the graph repository, CLI, and documentation consistent.

## Prerequisites

- Go **1.26.1** or later
- A checkout containing [`go.work`](go.work)

The workspace has two modules:

- [`./`](.) (`github.com/autonomous-bits/spool`) contains the graph repository and query
  packages.
- [`cmd/spl`](cmd/spl) (`github.com/autonomous-bits/spool/cmd/spl`) contains the Cobra CLI and
  depends on the root module through a local `replace` directive.

## Build and test

Run these commands from the repository root:

```sh
make check   # module tidiness, format, vet, static analysis, tests, race tests, and vulnerability scan
make build   # run the quality gate and build dist/spl
make fuzz    # run the bounded persisted-state fuzz target
make bench   # run benchmarks
make stress  # run opt-in repository stress tests
make tidy    # synchronize the workspace and both modules
```

`make stress` enables the opt-in `TestStress` tests, disables Go test caching, and uses a
stress-appropriate timeout. It is not included in `make check` or regular CI. By default, the
stress tests use 1,000 nodes, 5,000 edges, and 25 commits, as defined in
[`internal/repository/stress_test.go`](internal/repository/stress_test.go). Override these values
with the `SPOOL_STRESS_NODES`, `SPOOL_STRESS_EDGES`, and `SPOOL_STRESS_COMMITS` environment
variables; each override must be a positive integer. `SPOOL_STRESS_TIMEOUT` overrides the
default 10-minute test timeout for larger profiles.

The full quality gate uses pinned versions of Staticcheck, GolangCI-Lint, and `govulncheck`.
Install them when needed:

```sh
go install honnef.co/go/tools/cmd/staticcheck@v0.7.0
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0
go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
```

GitHub Actions runs `make check` and the bounded fuzz target for every push and pull request.

## Releases

`main` is protected, so prepare each release in a pull request:

1. Create a branch from `main`, move user-facing entries from [`CHANGELOG.md`](CHANGELOG.md)'s
   `Unreleased` section into a versioned section, and open a pull request.
2. Have the pull request approved and merged into `main`.
3. Create and push an annotated semantic-version tag for the resulting `main` commit:

   ```sh
   git fetch origin main
   git tag -a v0.1.0 origin/main -m "Release v0.1.0"
   git push origin v0.1.0
   ```

Pushing a `v*` tag runs the release workflow. GoReleaser cross-compiles `spl`, uploads archives
and `checksums.txt` to GitHub Releases, and generates release notes from eligible commits. It does
not modify `main` or `CHANGELOG.md`.

The workflow uses GitHub's automatic `GITHUB_TOKEN`; no repository secret is needed. In GitHub
repository settings, allow workflows **Read and write permissions**. The user or automation that
pushes the release tag must have Contents write permission and must be allowed by any `v*` tag
protection ruleset.

## Code conventions

- Define CLI subcommands in [`cmd/spl/commands`](cmd/spl/commands), wire them in
  [`cmd/spl/root.go`](cmd/spl/root.go), emit successful results as JSON to standard output, and
  return errors for `main` to log to standard error.
- Keep command validation in Cobra and put command-specific tests alongside the matching command.
- Keep repository lifecycle, durable storage, locking, and recovery in
  [`internal/repository`](internal/repository). The `branch`, `initialization`, `merge`, and
  `resolve` packages own their corresponding services and queries.
- Add idiomatic Go doc comments to exported APIs, including their validation, error, persistence,
  ownership, and concurrency contracts where relevant.
- Preserve the existing JSON request and response contracts unless the accompanying documentation
  and tests are updated intentionally.

See [`cmd/spl/AGENTS.md`](cmd/spl/AGENTS.md) and [`internal/AGENTS.md`](internal/AGENTS.md) for
the detailed package guidance.

## Documentation

Update user-facing command help and [`README.md`](README.md) when a CLI workflow changes. Use
[`docs/README.md`](docs/README.md) as the index for maintained project documentation.

The HTML and JSON files under [`docs/goals`](docs/goals) are generated delivery artifacts. Do not
hand-edit them without the corresponding generation source or process.
