# Spool

Spool is a local, content-addressed version-control system for graph data. It keeps immutable
snapshots of nodes and edges, lets you stage and commit graph mutations, and provides local
branching, history, and snapshot comparison through the `spl` command-line interface.

It is designed for machine integration: successful commands emit JSON to standard output, while
failures are structured JSON logs on standard error.

## Install

Spool currently builds from source and requires Go **1.26.1** or later:

```sh
go install github.com/autonomous-bits/spool/cmd/spl@latest
```

To build the checked-out workspace instead:

```sh
go build -o dist/spl ./cmd/spl
```

Prebuilt archives for released versions are available from the
[GitHub Releases](https://github.com/autonomous-bits/spool/releases) page. Verify a download with
the accompanying `checksums.txt` file.

## Quick start

Initialize a graph repository in your workspace:

```sh
spl init
```

This creates a `.spl` state directory with a default `main` branch. From a subdirectory, `spl`
locates the nearest parent `.spl` directory; when initializing, it uses the directory containing
`go.work`, or the current directory if none is found.

## Storage and integrity

Spool stores immutable nodes, edges, graph snapshots, schemas, and fixed-fanout
sorted tree indexes as canonical CBOR loose objects under `.spl/objects/loose`.
Object IDs are BLAKE3 hashes of the typed canonical bytes. Mutable control
state is separate: `.spl/config.toml`, `HEAD`, branch refs, staging files,
reflogs, and merge transactions.

`spl gc` retains reachable and reflog-referenced objects, packs retained objects
into verified zstd pack/index generations, and removes unreachable loose objects
only after a 14-day grace period. Pack publication is atomic, so packing does
not change object IDs or make a committed object unavailable.

Commit and merge transitions write immutable objects before atomically replacing
the affected ref. If a process stops during a transition, an unreachable object
or stale staging file may remain, but a ref never intentionally points to a
partially written object. Do not edit files inside `.spl` manually.

Use `fsck` after an interrupted process, storage failure, or suspected
corruption:

```sh
spl fsck
spl gc
```

It writes a JSON integrity report on standard output even when corruption is
found, exits non-zero for corruption, and does not repair or delete data.

Stage a JSON mutation batch and commit it:

```sh
spl add --branch main --batch mutations.json
spl status --branch main
spl commit --branch main --author alice --message "Add graph data"
```

Author a schema in TOML and stage its migration with the graph changes needed
to satisfy it:

```toml
# people.toml
version = 2

[[node]]
label = "Person"
[[node.property]]
key = "name"
required = true
types = ["string"]
```

```sh
spl schema migrate --branch main --schema people.toml --batch people-mutations.json
spl commit --branch main --author alice --message "Migrate people schema"
spl validate --branch main
```

`schema migrate` reads the TOML schema and the complete JSON mutation batch,
validates their resulting graph together, and atomically replaces the branch's
staged set. The schema and graph changes take effect together only when that
staged set is committed. `validate` emits a JSON report for one immutable
branch snapshot; use `--commit <commit-id>` to validate a reachable historical
commit instead of the branch head.

Create and use a branch:

```sh
spl branch create feature --from-branch main
spl switch feature
spl branch list
```

Query, retrieve, and compare graph snapshots:

```sh
spl resolve --branch main --node 11111111-1111-4111-8111-111111111111
spl diff --base-branch main --target-branch feature
spl history --branch main --entity-id 11111111-1111-4111-8111-111111111111
spl branches-containing --entity-id 11111111-1111-4111-8111-111111111111

# Lexical retrieval and schema-indexed metadata filters use the current
# branch-head SQLite projection.
spl search --branch main --query incident
spl filter --branch main --label Task --property-text status=open

# Build a bounded evidence context from lexical results or typed filters.
spl context --branch main --query incident --direction both --edge-type RELATES_TO
spl search-expand --branch main --label Task --property-min priority=3 --direction out
```

`search`, `filter`, `search-expand`, and `context` return the selected snapshot and projection
provenance with budget and completion metadata. Filtered properties must be scalar properties
enabled with `indexed = true` in the selected schema. Retrieval is currently limited to the
branch-head projection; selecting a historical commit is rejected. Use `--max-rows`,
`--max-response-bytes`, `--timeout`, `--max-visited`, and `--max-depth` to narrow the configured
query limits.

Run `spl <command> --help` for all commands, flags, response-budget controls, and examples.

Preview a merge before moving a branch, then apply the exact clean preview:

```sh
spl merge preview --source feature --target main
spl merge apply --source feature --target main --transaction merge-42 --preview <preview-id> \
  --author alice --message "Merge feature"
```

The preview combines independent node/edge fields and property keys, and reports structural,
schema, and schema-derived semantic conflicts with stable conflict IDs and affected paths.
Applying a conflicted exact preview creates a durable, target-branch lease rather than moving the
branch. Inspect it with `spl merge conflicts --target main --transaction merge-42`, resolve every
conflict from a JSON selection array, then finalize or abort:

```sh
spl merge resolve --target main --transaction merge-42 --preview <preview-id> \
  --selections selections.json [--overrides mutations.json]
spl merge finalize --target main --transaction merge-42
# or: spl merge abort --target main --transaction merge-42
```

Each selection is `{"conflictId":"...","choice":"source"}` or
`{"conflictId":"...","choice":"target"}`. Overrides use the same
mutation-array format as `spl add` and can repair a schema-derived semantic conflict. Resolution
and finalization reject stale previews, require transaction ownership, and keep the target lease
until finalization or abort.

## Learn more

- [`CONTRIBUTING.md`](CONTRIBUTING.md) explains how to build, test, and contribute to Spool.
- [`docs/architecture.md`](docs/architecture.md) describes the high-level system architecture.
- [`CHANGELOG.md`](CHANGELOG.md) explains how release notes are generated.
