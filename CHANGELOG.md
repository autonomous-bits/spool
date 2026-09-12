# Changelog

All notable changes to Spool are documented in GitHub Releases.

This project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Release notes are
generated from commits since the preceding `v*` tag. Commits prefixed with `docs:`, `test:`,
`chore:`, `ci:`, `build:`, or Dependabot's `Bump ` prefix are excluded from release notes.

## [Unreleased]

## [1.8.0] - 2026-09-12

### Added

- Contextual reference asset storage and CLI commands for managing assets associated with graph
  entities, with remote synchronization support.

## [1.7.0] - 2026-09-09

### Added

- Support for non-linear commit DAG histories (including merge commits with multiple parents)
  in `spl push` and `spl pull`:
  - `spl push` performs iterative post-order topological traversal to resolve and push multi-parent
    commit graphs to Spool Rack remotes, negotiating `PackFormatV3` when merge commits are present
    while retaining backwards-compatible `PackFormatV2` for linear histories.
  - `spl pull` unpacks and connects multi-parent DAG commit chains, resolving parent commits cleanly
    across converging branches.

## [1.6.0] - 2026-09-07

### Added

- `spl clone` CLI command (and `spl workspace clone`) that clones a remote Spool Rack
  workspace into a new local directory over the native clone/pull protocol. It supports both
  positional URL syntax (`spl clone <url> [dir]`) and flag syntax (`spl clone --endpoint <url> --workspace-id <id> [dir]`),
  authenticates via Bearer or Basic auth, initializes the local repository with remote
  configuration and remote-tracking branch state, verifies and installs packs from scratch, and
  materializes initial graph projections ready for immediate mutations and commits.

## [1.5.0] - 2026-09-07

### Added

- `spl migrate` CLI command (and `spl workspace migrate`) that upgrades repository
  state between format versions (e.g. `spl migrate --from 1 --to 2`). When opening
  a repository whose format cannot be read because it is from an older format version,
  Spool halts with an actionable error instructing the user to run `spl migrate --from <from> --to <to>`.
  The migration creates a durable, timestamped backup (`.v1.backup-<timestamp>`), canonicalizes the
  commit DAG using `graphcontract.Commit`, remaps all branch refs and reflogs, updates `config.toml`,
  invalidates SQLite projections, and verifies repository integrity.
- `spl pull` CLI command that fetches new commits for a branch from a
  repository's configured Rack remote over the native pull protocol: it
  recomputes the local branch's Rack wire-format head, asks Rack for
  anything newer, and installs any new commits as a fast-forward extension
  of local history, preserving each pulled commit's original author,
  message, and time. It reports a clean "up to date" result when local
  history already matches Rack's head, and reports divergence (Rack's head
  is not a descendant of the local branch) without attempting a merge.
  Bootstrapping a branch with no local history in common with Rack is not
  yet supported.
- `spl push` CLI command that pushes verified local native commits and packs
  to a repository's configured Rack remote over the native (non-JSON) push
  protocol: it recomputes the Rack wire-format commit chain from local
  history, builds a canonical CBOR pack, and sends it as a multipart
  request, reporting a clean "nothing to push" result or Rack's non-fast-
  forward rejection without attempting to merge or retry. Only linear,
  fast-forward push ranges are supported; merge-commit and remote-branch-
  tracking support are tracked separately.
- `spl push --reconcile` flag that automatically handles a non-fast-forward
  push rejection: it fetches Rack's complete current history for the
  branch, rebases the branch's independent local changes onto it with the
  graph merge engine as a single-parent, fast-forward-eligible commit, and
  retries the push. Merge conflicts leave both the branch and a dedicated
  `reconcile/<branch>` branch untouched and are reported instead of
  retried, so they can be resolved with the existing `spl merge`
  subcommands before retrying.
- Public `graphcontract` merge APIs exposing deterministic three-way graph
  merge simulation (`ThreeWayMerge`, `MergeResult`, `MergeChange`,
  `MergeConflict`, and conflict finalization helpers), enabling downstream
  integrations to share Spool's stable merge conflict ordering, identifiers,
  and paths without depending on `internal/repository`.
- Public `graphcontract.Snapshot` type exposing the canonical graph snapshot
  root (node, edge, incoming-adjacency, outgoing-adjacency, and schema tree
  roots, plus entity counts), with `NewSnapshot`, `MarshalSnapshot`, and
  `UnmarshalSnapshot` canonical CBOR helpers, enabling downstream
  integrations to interpret Spool snapshot roots without depending on
  `internal/repository`.
- Public `graphcontract` pack APIs exposing the immutable pack container
  format (`PackID`, `PackManifest`, `PackIndexEntry`, `PackHeader`, and
  related errors) and full packed-object integrity verification
  (`ValidatePackHeader`, `ValidatePackManifest`, `ValidatePackIndexEntry`,
  `ValidatePackEntries`, `DecompressPackedObject`), enabling downstream
  integrations to verify native Spool packs without depending on
  `internal/repository`.
- Versioned `graphcontract` conformance fixtures for canonical objects
  (`testdata/objects/v1/`) and commits (`testdata/commits/v1/`), plus
  invalid-pack fixtures (`testdata/pack/v1/invalid-*.json`) and a
  `testdata/MANIFEST.json` index enumerating every fixture set. Together
  with the existing schema fixtures, this proves canonical `Node`, `Edge`,
  `Commit`, pack, and schema encoding, decoding, and stable error behavior
  for both valid and invalid inputs, giving Rack a single, language-agnostic
  fixture surface to prove graph-contract parity before upgrading its
  pinned Spool dependency.

## [2.1.0] - 2026-09-06

### Added

- Public `graphcontract` schema APIs for canonical schema CBOR, TOML parsing
  and normalization, schema validation, and stable normalized violations,
  enabling downstream integrations to share Spool's schema semantics.

## [1.4.0] - 2026-09-06

### Added

- Public `graphcontract` schema APIs for canonical schema CBOR, TOML parsing
  and normalization, schema validation, and stable normalized violations are
  available on the original Go module path for downstream integrations.

## [2.0.0] - 2026-09-05

### Changed

- Commit objects now use the public `graphcontract` canonical CBOR contract.
  Existing repository state is intentionally unsupported because commit object
  identifiers may differ.

## [1.3.0] - 2026-09-05

### Added

- Public `graphcontract` package providing the canonical graph domain types and
  deterministic CBOR serialization for downstream Go integrations.

## [1.2.0] - 2026-08-30

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
