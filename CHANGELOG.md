# Changelog

All notable changes to Spool are documented in GitHub Releases.

This project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Release notes are
generated from commits since the preceding `v*` tag. Commits prefixed with `docs:`, `test:`,
`chore:`, `ci:`, `build:`, or Dependabot's `Bump ` prefix are excluded from release notes.

## [Unreleased]

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
