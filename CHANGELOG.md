# Changelog

All notable changes to Spool are documented in GitHub Releases.

This project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Release notes are
generated from commits since the preceding `v*` tag. Commits prefixed with `docs:`, `test:`,
`chore:`, `ci:`, `build:`, or Dependabot's `Bump ` prefix are excluded from release notes.

## [Unreleased]

### Added

- `spl cherry-pick` command to selectively transplant individual historical commit deltas onto a target branch with dry-run preview, 3-way property merging, and strict referential integrity preflight validation.

## [1.1.0] - 2026-08-30

### Added

- `spl version` command to display application release version, git commit hash,
  build timestamp, Go runtime version, and platform as structured JSON.

## [1.0.0] - 2026-08-30

### Changed

- Workspace setup now uses explicit portable repository manifests and central
  detached workspace state instead of legacy host-path and active-workspace
  resolution.

## [0.0.8] - 2026-08-30

### Added

- Portable workspace manifests for reproducible multi-repository workspace setup.

## [0.0.7] - 2026-08-30

### Added

- Prebuilt `spl` binaries for macOS, Linux, and Windows are available from
  GitHub Releases.

## [0.0.6] - 2026-08-28

### Added

- `spl prune` removes ephemeral entities and their cascading edges to simplify
  repository maintenance.

## [0.0.5] - 2026-08-23

### Changed

- Improved performance by batching immutable commit objects in pack files.

## [0.0.4] - 2026-08-23

### Changed

- Improved performance when accessing packed repository objects.

## [0.0.3] - 2026-08-23

### Changed

- Folded `spl init` into `spl workspace init` for unified workspace initialization.

## [0.0.2] - 2026-08-22

### Added

- Multi-repo Spool workspaces with a registry, `spl workspace` CLI commands,
  and discovery overrides.
- `spl workspace use` and `spl workspace unset` for persisted workspace
  selection.
- `spl graph` command with pinned-node support for graph retrieval.
- An interactive 3D graph canvas extension for visualizing branch graphs.

## [0.0.1] - 2026-08-20

### Added

- Typed property-graph snapshots with labels, typed properties, schema policy,
  validation, and schema migrations.
- Canonical CBOR/BLAKE3 loose-object storage, immutable pack generations,
  `spl gc`, and read-only `spl fsck` integrity reporting.
- Deterministic three-way merge previews, clean merge applies, and durable
  conflict inspection, resolution, finalization, and abort workflows.
- A versioned SQLite/FTS5 branch-head projection with lexical search,
  schema-indexed filters, bounded graph expansion, and evidence-focused context
  assembly.
- Strict snapshot selection and configurable limits for query rows, response
  size, traversal depth, visited nodes, and execution time.
