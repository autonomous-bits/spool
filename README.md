# Spool

Spool is a local, content-addressed version-control system for graph data. It keeps immutable
snapshots of nodes and edges, lets you stage and commit graph mutations, and provides local
branching, history, and snapshot comparison through the `spl` command-line interface.

It is designed for machine integration: successful commands emit JSON to standard output, while
failures are structured JSON logs on standard error.

## Install

### Prebuilt binaries

Download the latest prebuilt archive for your platform from [GitHub Releases](https://github.com/autonomous-bits/spool/releases):

| Platform | Architecture | Archive |
| :--- | :--- | :--- |
| **macOS** | Apple Silicon (`arm64`) | `spool_<version>_darwin_arm64.tar.gz` |
| | Intel (`amd64`) | `spool_<version>_darwin_amd64.tar.gz` |
| **Linux** | ARM64 (`arm64`) | `spool_<version>_linux_arm64.tar.gz` |
| | x86-64 (`amd64`) | `spool_<version>_linux_amd64.tar.gz` |
| **Windows** | x86-64 (`amd64`) | `spool_<version>_windows_amd64.zip` |

Extract the archive and place the `spl` binary (or `spl.exe` on Windows) into a directory on your `PATH`. You can verify downloads with the accompanying `checksums.txt` file.

### Package managers (Upcoming)

Distribution through package managers is planned for upcoming releases:

- **Homebrew** (macOS / Linux): *Coming soon*
- **WinGet** (Windows): *Coming soon*

### Go install

If you have Go **1.26.1** or later installed:

```sh
go install github.com/autonomous-bits/spool/cmd/spl@latest
```

### Build from source

To build from a local clone of the repository:

```sh
git clone https://github.com/autonomous-bits/spool.git
cd spool
go build -o dist/spl ./cmd/spl
# or: make build
```

## Quick start

Create a detached workspace and attach the checkout that will use it:

```sh
spl workspace init my-project
spl workspace attach --workspace my-project .
```

`spl workspace init` creates the workspace's graph state and default `main` branch in detached
storage immediately, not in the checkout, so the workspace is usable as soon as a checkout is
attached. Every path attached to that workspace uses the same state; attach additional checkouts
directly:

```sh
spl workspace attach --workspace my-project ~/repos/my-project-docs
```

Confirm which workspace owns the current directory with:

```sh
spl workspace current
```

To create a local, non-workspace graph repository instead, run `spl init` from the desired
directory. It creates `.spl` in the nearest directory at or above the current directory that
already contains `.spl` or `go.work`, or in the current directory if neither is found.

All commands accept the global `--state-dir <path>` override. State selection precedence is
`--state-dir`, `SPOOL_DIR`, `SPOOL_WORKSPACE`, the persisted workspace preference, the registered
workspace owning the current path (longest match), then local `.spl`/`go.work` discovery. An empty
`--state-dir` is invalid; a stale persisted workspace preference is ignored so `spl workspace unset`
can recover it.

## Storage and integrity

Spool stores immutable nodes, edges, graph snapshots, schemas, and fixed-fanout sorted tree
indexes as canonical CBOR loose objects under the selected state directory's `objects/loose`.
For local repositories, the state directory is `.spl`; for attached workspaces, it is detached
storage outside the checkout.
Object IDs are BLAKE3 hashes of the typed canonical bytes. Mutable control
state is separate: `config.toml`, `HEAD`, branch refs, staging files,
reflogs, and merge transactions.

`spl gc` retains reachable, reflog-referenced, and durable merge-resolution root objects, packs
retained objects into verified zstd pack/index generations, and removes unreachable loose objects
only after a 14-day grace period. Pack publication is atomic, so packing does
not change object IDs or make a committed object unavailable.

Commit and merge transitions write immutable objects before atomically replacing
the affected ref. If a process stops during a transition, an unreachable object
or stale staging file may remain, but a ref never intentionally points to a
partially written object. Do not edit files in the selected Spool state directory manually.

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

Remove temporary planning data before merging a feature branch:

```sh
# Inspect the affected nodes, incident edges, and durable nodes left disconnected.
spl prune --branch feature --dry-run

# Remove nodes labeled Ephemeral and commit the resulting graph snapshot when any are found.
spl prune --branch feature --author alice --message "Prune transient plan"
```

`prune` emits a JSON summary containing the removed node and cascading-edge counts, node IDs, and
any durable nodes left without connected edges. It refuses to run when the branch has staged
changes; commit or clear them first. The default branch is protected from pruning unless
`--force` is explicitly supplied.

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
branch-head projection; a `--commit` selector is accepted only when it names that branch head,
and historical or divergent commits are rejected. `search` and `filter` (plus `history`,
`branches-containing`, and `diff`) support `--continuation`; `search-expand` and `context` do not.
Use `--max-rows`, `--max-response-bytes`, and `--timeout` for read budgets; `resolve`,
`search-expand`, and `context` additionally accept `--max-visited` and `--max-depth`.
`search-expand` and `context` require exactly one lexical `--query` or one or more typed filter
flags, and accept `--seed-limit`, repeatable `--edge-type`, and `--direction out|in|both`.

Export a branch's complete immutable graph snapshot as JSON for visualization or offline
inspection:

```sh
spl graph --branch main
```

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

## Multi-repo workspaces

A detached workspace maps one or more checked-out paths to the same Spool graph state. Create the
workspace and attach a path; the workspace's state is initialized immediately:

```sh
spl workspace init ecommerce-platform
spl workspace attach --workspace ecommerce-platform ~/repos/order-service
spl workspace attach --workspace ecommerce-platform ~/repos/inventory-service
spl workspace list
spl workspace current
spl workspace detach ~/repos/inventory-service
```

A repository path can be attached to only one workspace at a time. `spl workspace current`
resolves the registered workspace that owns the current working directory by longest matching
attached path. Workspace registry and state data live under the platform-appropriate XDG data
location (or `%LOCALAPPDATA%` on Windows), outside the attached checkouts. Persist or clear a
preferred active workspace for future sessions with:

```sh
spl workspace use ecommerce-platform
spl workspace unset
```

`unset` always succeeds, including when no preference is set, so it also recovers from a stale
preference pointing at a workspace no longer in the registry.

## CLI command reference

The complete installed surface, including generated help and every flag, is documented in
[`.agents/skills/spool/references/cli-help.md`](.agents/skills/spool/references/cli-help.md).
The command and flag inventory is:

| Command | Flags and positional arguments |
| --- | --- |
| `init` | none |
| `add` | `--branch` (required), `--batch` (required) |
| `status` | `--branch` |
| `commit` | `--branch` (required), `--author`, `--message` |
| `branch create <name>` | exactly one of `--from-branch`, `--from-commit` |
| `branch list` | none |
| `branch delete <name>` | none; branch must be inactive and non-default |
| `switch <branch>` | positional branch |
| `schema migrate` | `--branch`, `--schema`, `--batch` (all required) |
| `validate` | `--branch` (required), `--commit` (reachable) |
| `resolve` | `--branch` (required), `--commit`, `--node`, `--max-rows`, `--max-response-bytes`, `--timeout`, `--max-depth`, `--max-visited` |
| `graph` | `--branch` (required) |
| `search` | `--branch`, `--query` (required), `--commit`, `--continuation`, `--max-rows`, `--max-response-bytes`, `--timeout` |
| `filter` | `--branch` (required), `--commit`, repeatable `--label`/property predicates, `--continuation`, `--max-rows`, `--max-response-bytes`, `--timeout` |
| `search-expand`, `context` | `--branch` (required), `--commit`, query or typed filters, `--direction`, repeatable `--edge-type`, `--seed-limit`, `--max-depth`, `--max-visited`, `--max-rows`, `--max-response-bytes`, `--timeout` |
| `history` | `--branch`, `--entity-id` (required), `--commit`, `--all-parents`, `--continuation`, `--max-rows`, `--max-response-bytes`, `--timeout` |
| `branches-containing` | exactly one selector (`--entity-id`, `--snapshot-id`, `--natural-key`), `--continuation`, `--max-rows`, `--max-response-bytes`, `--timeout` |
| `diff` | `--base-branch`, `--target-branch` (required), optional commits, repeatable `--node-id`/`--edge-id`, `--node-title-contains`, `--one-hop`, `--continuation`, `--max-rows`, `--max-response-bytes`, `--timeout` |
| `merge preview` | `--source`, `--target` (required) |
| `merge apply` | `--source`, `--target`, `--transaction`, `--preview` (required), `--author`, `--message` |
| `merge conflicts` | `--target`, `--transaction` (required) |
| `merge resolve` | `--target`, `--transaction`, `--preview`, `--selections` (required), `--overrides` |
| `merge finalize`, `merge abort` | `--target`, `--transaction` (required) |
| `fsck` | none; read-only integrity report |
| `gc` | `--dry-run`, `--repack`, `--grace-period` (default `336h`) |
| `prune` | `--branch` (required), `--dry-run`, `--force`, `--author`, `--message` |
| `workspace init <name>` | positional name |
| `workspace attach [path]` | `--workspace` (required); path defaults to current directory |
| `workspace detach <path>` | positional path |
| `workspace list`, `workspace current`, `workspace unset` | none |
| `workspace use <name>` | positional name |
| `completion` | shell subcommand: `bash`, `zsh`, `fish`, or `powershell` |
| `help [command path]` | optional command path |

The common query-budget flags are `--max-rows`, `--max-response-bytes`, and `--timeout`;
traversal commands additionally use `--max-depth` and `--max-visited` as listed in the full
reference. Every command also accepts `-h, --help` and the global `--state-dir`.

## Learn more

- [`CONTRIBUTING.md`](CONTRIBUTING.md) explains how to build, test, and contribute to Spool.
- [`docs/architecture.md`](docs/architecture.md) describes the high-level system architecture.
- [`CHANGELOG.md`](CHANGELOG.md) explains how release notes are generated.
