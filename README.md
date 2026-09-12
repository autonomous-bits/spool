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

Create a central detached workspace, then explicitly bind each repository:

```sh
spl workspace init my-project
spl workspace attach --workspace my-project --repository-id github.com/acme/my-project .
```

`workspace init` creates the detached state in the user's central workspace
catalog. `workspace attach` writes the checkout's portable `.spl/config.toml`
manifest, binding its repository ID to the workspace's immutable ID. Commit the
manifest so clones, worktrees, and CI runners resolve the same state. Repeat
`workspace attach` for each repository that belongs to the workspace. It does
not persist host-path attachments. If the checkout already uses
`.spl/config.toml` for local Spool state, do not overwrite it with a workspace
manifest. Repositories that ignore `.spl` must add a `.gitignore` exception for
`.spl/config.toml` before committing the manifest.

To create a local, non-workspace graph repository instead, run `spl init` from the desired
directory. It creates `.spl` in the nearest directory at or above the current directory that
already contains `.spl` or `go.work`, or in the current directory if neither is found.

All commands accept the global `--state-dir <path>` override. State selection
precedence is `--state-dir`, `SPOOL_DIR`, a discovered checkout manifest, then
local `.spl`/`go.work` discovery. An empty `--state-dir` is invalid. A malformed
manifest or unregistered workspace ID is an error; checkouts without a manifest
continue to use local repository discovery.

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

Selectively transplant an individual commit's graph delta onto a target branch:

```sh
# Preview the transplanted changes, conflicts, and schema violations without modifying state.
spl cherry-pick --commit <commit-id> --target-branch main --dry-run

# Apply the exact single-commit delta onto main with provenance metadata.
spl cherry-pick --commit <commit-id> --target-branch main --author alice --message "Transplant fix"
```

`cherry-pick` computes the single-commit delta against its first parent, performs 3-way property merging
against the target branch head, and strictly verifies referential integrity and schema conformance
before committing.

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

Store contextual reference documents as content-addressed Asset nodes:

```sh
spl asset add --branch main --file docs/architecture.md --title "Architecture notes"
spl commit --branch main --author alice --message "Add architecture notes"
spl asset read --node asset-<hash-prefix> --branch main > architecture.md
```

`asset add` stores the file under `.spl/assets/loose`, computes its BLAKE3-256 hash and MIME
type, and stages an `Asset` node with a `spool://assets/<hash>` locator. `asset read` accepts an
Asset node ID, a locator, or a raw hash and streams the original bytes without buffering the full
file. When a configured Rack remote has the blob but the local cache does not, `asset read`
retrieves and caches it on demand. `spl push` negotiates missing asset blobs and uploads them
alongside the graph history.

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

## Detached workspaces

`workspace init` provisions a central detached workspace. `workspace attach`
explicitly writes a repository manifest for it. Repository resolution uses only
that committed manifest's immutable `workspace_id`; it does not use host-path
attachments, `SPOOL_WORKSPACE`, or active-workspace preferences.

## Remote configuration and synchronization

Spool connects to a remote Spool Rack service to push, pull, and coordinate graph history across team members.

### Remote configuration

`remote set` persists a non-secret Rack remote configuration to `.spl/config.toml`:

```sh
# Configure with tenant and workspace identity
spl remote set --endpoint https://rack.example.com --tenant-id acme --workspace-id prod --auth-mode bearer

# Or using legacy repository identity
spl remote set --endpoint https://rack.example.com --repo-id acme-prod --auth-mode bearer
```

`remote show` reports the configured remote and probes its `/healthz` endpoint to verify `graphcontract` version compatibility:

```sh
spl remote show
```

`remote remove` clears the configured remote from `.spl/config.toml`:

```sh
spl remote remove
```

No credential is ever read from or written to repository configuration — credentials are resolved at use from the OS keychain/secret store (service `spool-rack`, account = workspace/repo-id), then `SPOOL_RACK_TOKEN` / `SPOOL_RACK_API_KEY`, then an interactive prompt.

### Cloning a remote workspace

To clone an existing remote workspace hosted on Spool Rack into a new local directory:

```sh
# Clone by workspace URL
spl clone http://127.0.0.1:8080/api/v1/workspaces/ws-backend [directory]

# Or clone by parameters
spl clone --endpoint http://127.0.0.1:8080 --tenant-id acme --workspace-id ws-backend [directory]

# Also available under the workspace command group
spl workspace clone http://127.0.0.1:8080/api/v1/workspaces/ws-backend [directory]
```

`clone` initializes the local workspace directory, configures the Rack remote, fetches the complete graph history for the default branch (or specified `--branch`), and makes it the active branch so you can immediately begin pulling, committing, and pushing ideas.

### Remote branches

Inspect and manage branch lifecycle on the configured Rack remote:

```sh
# Create a remote branch from an existing remote branch or commit
spl remote branch create feature --from-branch main
spl remote branch create review --from-commit <commit-id>

# List remote branches or query the remote default branch
spl remote branch list
spl remote branch default

# Delete a non-default remote branch
spl remote branch delete feature
```

### Push and pull

Exchange verified commits with the configured Rack remote over native pack protocols:

```sh
# Push local commits reachable from --branch to Rack
spl push --branch main

# Push only commits newer than a known remote base commit
spl push --branch main --base-commit <last-known-wire-commit-id>

# Automatically reconcile non-fast-forward rejections via 3-way graph merge and retry
spl push --branch main --reconcile

# Pull new commits from Rack and fast-forward local history
spl pull --branch main
```

`push` and `pull` enforce linear, fast-forward history. If a push is rejected because the remote branch moved, `--reconcile` pulls the remote history into a temporary reconciliation branch, rebases local changes onto it using the 3-way graph merge engine, and retries the push if clean. If conflicts exist, they are reported so they can be inspected and resolved using `spl merge`.

## Workspace format migration

When upgrading Spool across format version increments (such as upgrading from format version 1 to format version 2 in v1.5.0), existing workspace state must be migrated before it can be read or modified:

```sh
# Upgrade repository state directory format
spl migrate --from 1 --to 2

# Also available under the workspace command group
spl workspace migrate --from 1 --to 2
```

`migrate` acquires an exclusive lock on repository control state, creates a durable backup copy of the state directory (e.g. `.v1.backup-<timestamp>`), canonicalizes commit objects to current graph contracts, remaps references and reflogs, updates configuration format version and tracking metadata, and validates the upgraded repository with `fsck`.

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
| `cherry-pick` | `--commit` (required), `--target-branch` (required), `--dry-run`, `--author`, `--message` |
| `asset add` | `--branch` and `--file` (required), `--title`, `--id` |
| `asset read` | positional locator/node ID or `--locator`/`--node`, `--branch` |
| `workspace init <name>` | positional name |
| `workspace attach [path]` | `--workspace` and `--repository-id` (required); path defaults to current directory |
| `workspace migrate`, `migrate` | `--from` (required), `--to` (required) |
| `remote set` | `--endpoint` (required), `--auth-mode` (required), `--workspace-id`/`--workspace` or `--tenant-id`/`--tenant` or legacy `--repo-id` |
| `remote show` | none; probes the configured remote's `/healthz` and never prints credentials |
| `remote remove` | none |
| `remote branch create <name>` | exactly one of `--from-branch`, `--from-commit` |
| `remote branch list` | none |
| `remote branch default` | none |
| `remote branch delete <name>` | positional branch name (cannot delete default remote branch) |
| `clone`, `workspace clone` | positional `[url]` or flags (`--endpoint`, `--workspace-id`/`--workspace`, `--tenant-id`/`--tenant`), optional `[directory]`, `--branch`/`-b`, `--auth-mode` |
| `push` | `--branch` (required), `--base-commit`, `--reconcile` |
| `pull` | `--branch` (required) |
| `version` | none |
| `completion` | shell subcommand: `bash`, `zsh`, `fish`, or `powershell` |
| `help [command path]` | optional command path |

The common query-budget flags are `--max-rows`, `--max-response-bytes`, and `--timeout`;
traversal commands additionally use `--max-depth` and `--max-visited` as listed in the full
reference. Every command also accepts `-h, --help` and the global `--state-dir`.

## Learn more

- [`CONTRIBUTING.md`](CONTRIBUTING.md) explains how to build, test, and contribute to Spool.
- [`docs/architecture.md`](docs/architecture.md) describes the high-level system architecture.
- [`CHANGELOG.md`](CHANGELOG.md) explains how release notes are generated.
