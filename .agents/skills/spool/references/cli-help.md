# CLI help

`spl` is a JSON-oriented CLI: successful command results are written to stdout. The executable
logs failures as structured JSON on stderr and exits non-zero. Help itself is human-readable.

Inspect the installed command surface and flags instead of assuming a release supports a feature:

```sh
spl --help
spl <command> --help
spl merge --help
```

## Repository selection

Solution context is selected only from an explicit `.spool/context.toml` in the
code repo (see [docs/context-bind.md](../../../../docs/context-bind.md)). Remotes are
never inferred from directory names, monorepo layout, `go.work`, leftover `.spl`
state, git `origin`, or Rack config. Bind + stock git is the **only** durable SoT.
Every agent write is a short-lived branch + PR.

Every command also accepts the persistent global flag for leftover local `.spl`
state used by migrate-once export. That path is **deprecated / unsupported** as
a solution-context store:

```text
--state-dir <path>  override leftover local `.spl` state (migration-only)
```

Before Cobra dispatches a leftover-local command, that state directory is selected in this order:

1. `--state-dir <path>` (or `--state-dir=<path>`)
2. `SPOOL_DIR`
3. a validated ancestor `.spl/config.toml` workspace manifest
4. the nearest leftover `.spl` directory or `go.work` root, or `.spl` in the current directory

An empty `--state-dir` is invalid. A malformed workspace manifest or unknown
workspace ID is an error; checkouts without a workspace manifest use leftover
local discovery. When `.spool/context.toml` is present, `init`, `workspace`,
`remote`, `push`, `pull`, and `clone` refuse: those paths are deprecated and
unsupported for solution context.

## Command index

| Command | Purpose |
| --- | --- |
| `init` | **Deprecated / unsupported** for solution context; leftover `.spl` only |
| `add`, `status`, `commit` | Stage, inspect, and commit a complete mutation set (bound: short-lived branch + PR) |
| `branch create/list/delete`, `switch` | Manage local branches |
| `schema migrate`, `validate` | Stage schema migrations and validate snapshots |
| `resolve`, `graph` | Read a node or export a complete branch snapshot |
| `search`, `filter`, `search-expand`, `context` | Query the branch-head projection |
| `context init` | Bind this code repo to a solution context git remote and seed layout/`CodeRepository` |
| `context export` (`migrate-once`) | One-shot `.spl` → context git export (lossy; not sync) |
| `history`, `branches-containing`, `diff` | Inspect history, branch containment, and snapshot changes |
| `merge preview/apply/conflicts/resolve/finalize/abort` | Run the merge transaction lifecycle |
| `fsck`, `gc`, `prune` | Check integrity, maintain objects, and remove ephemeral graph data |
| `asset add/read` | Store contextual reference documents and stream content-addressed asset blobs |
| `workspace init/attach/migrate`, `migrate` | **Deprecated / unsupported** leftover `.spl` workspaces (refused when bound) |
| `remote set/show/remove/branch` | **Unsupported** Rack sync (sunset; refused when bound) |
| `push`, `pull`, `clone` | **Unsupported** Rack pack transport (sunset; refused when bound) |
| `version` | Print Spool release version and build information as JSON |
| `completion`, `help` | Generate shell completion and inspect command help |

All commands accept `-h, --help` in addition to the flags below.

## Working changes

```sh
spl context init --remote https://github.com/org/solution-context.git
spl add --branch main --batch mutations.json
spl status --branch main
spl commit --branch main --author alice --message "Add graph data"
```

Bound `add`/`commit` land as a short-lived branch + PR. `spl init` is
**deprecated and unsupported** for solution context. Leftover `.spl` is
migration-only (`spl context export`).

`add` requires `--branch` and `--batch`, validates a complete JSON mutation-operation array, and
atomically replaces that branch's staged set; it does not commit. Operations can add, update, or
delete nodes and edges. Node operations may carry `labels` and `properties`; edge operations may
carry `type` and `properties`. Property values are tagged recursively with `kind` (`null`, `bool`,
`integer`, `float`, `string`, `list`, or `map`).

```text
add
  --branch <name>  branch on which to stage the batch (required)
  --batch <path>   JSON mutation-operation array (required)
```

`status` reports the staged delta for `--branch` as JSON (an unstaged branch has an empty delta).
`commit` requires `--branch`; `--author` and `--message` provide commit metadata. A successful
commit clears staging. If the canonical transition committed but cleanup or durability reporting
fails, `commit` still writes its JSON result and exits with a durability warning.

```text
status
  --branch <name>  branch whose staged delta to report

commit
  --branch <name>   branch whose staged mutations to commit (required)
  --author <text>   commit author
  --message <text>  commit message
```

## Context bind

```sh
spl context init --remote https://github.com/org/solution-context.git \
  --solution-id my-solution \
  --repository-id github.com/org/svc-api
```

`context init` writes `.spool/context.toml` in the current code repo, creates
`schema.toml` / `nodes/` / `edges/` / `assets/` on an empty context remote with
stock git, and seeds a `CodeRepository` node from this bind. Repeat in each code
repo that shares the remote. Sibling directories are not scanned. MCP writes
**always** use a short-lived branch + PR (never push-clean to the protected
branch). Auth is stock git credentials only. See
[docs/context-bind.md](../../../../docs/context-bind.md).

```text
context init
  --remote <url>             context git remote URL (required; stock git credentials)
  --solution-id <id>         solution identifier (defaults to the current directory name)
  --protected-branch <name>  protected integration branch (default main)
  --repository-id <id>       code-repo namespace for node IDs
  --author <text>            git author for the layout commit
```

## Context export (migrate-once)

```sh
spl context export --branch main
spl context migrate-once --branch main
```

`context export` (alias `migrate-once`) is the documented **escape hatch**: a
best-effort **one-shot** mapping of leftover local `.spl` into the bound
context git remote. It is not sync, not dual-write, and not Rack wire-compat.
Kept: nodes, edges, schema, assets (LFS ≥512 KiB). Dropped: packs, Rack remotes,
reflogs, merge leases, projections. Success JSON lists kept vs skipped. Re-run
is overwrite-at-own-risk. Requires `.spool/context.toml`. Export opens one
short-lived branch and PR (gate G2); it fails closed if dropped paths would
land in the tree. See
[docs/context-git-migration.md](../../../../docs/context-git-migration.md).

```text
context export
  --branch <name>   local .spl branch (defaults to the active branch)
  --author <text>   git author for the export commit
  --message <text>  commit/PR title
```

## Branches and schemas

```sh
spl branch create feature --from-branch main
spl branch create review --from-commit <commit-id>
spl branch list
spl switch feature
spl branch delete feature
```

`branch create <name>` requires exactly one of `--from-branch` and `--from-commit`.
`branch delete <name>` only deletes an inactive, non-default branch.

```text
branch create <name>
  --from-branch <name>  existing branch source
  --from-commit <id>    existing commit source
branch list
branch delete <name>
switch <branch>
```

Stage a schema and its complete conforming graph mutation batch together:

```sh
spl schema migrate --branch main --schema people.toml --batch people-mutations.json
spl commit --branch main --author alice --message "Migrate people schema"
spl validate --branch main
spl validate --branch main --commit <reachable-commit-id>
```

`schema migrate` requires `--branch`, `--schema`, and `--batch`; rejected migrations leave the
previous staged set unchanged. `validate` requires `--branch`; its optional `--commit` must be
reachable from that branch.

```text
schema migrate
  --branch <name>  branch on which to stage the migration (required)
  --schema <path>  target TOML schema (required)
  --batch <path>   complete JSON mutation-operation array (required)

validate
  --branch <name>  branch to validate (required)
  --commit <id>    reachable commit to validate
```

## Reading graphs

```sh
spl resolve --branch main --node <node-id>
spl resolve --branch main --commit <reachable-commit-id> --node <node-id>
spl graph --branch main
```

`resolve` requires `--branch` and a stable `--node` ID; an optional `--commit` selects a reachable
historical snapshot. `graph` requires `--branch` and exports every node and edge from that branch
snapshot.

```text
resolve
  --branch <name>             branch to resolve (required)
  --commit <id>               reachable commit to resolve
  --node <id>                 stable node entity ID
  --max-rows <n>              maximum rows
  --max-response-bytes <n>    maximum response size
  --timeout <duration>        maximum query duration
  --max-depth <n>             maximum traversal depth
  --max-visited <n>           maximum visited nodes

graph
  --branch <name>  branch to export (required)
```

## Projection retrieval

```sh
spl search --branch main --query incident
spl filter --branch main --label Task --property-text status=open
spl filter --branch main --property-min priority=3
spl search-expand --branch main --query incident --direction out --edge-type RELATES_TO
spl context --branch main --label Task --property-text status=open --direction both
```

`search`, `filter`, `search-expand`, and `context` query only the selected branch-head SQLite/FTS5
projection. Their optional `--commit` is accepted only when it names the current branch head;
historical or divergent snapshots are rejected. `search` requires `--branch` and `--query`.
`filter` requires `--branch`; it accepts repeatable labels and indexed scalar property comparisons.
Each property comparison is `key=value`:

```text
--label <label>                 required node label (repeatable)
--property-text <key=value>     indexed text equality (repeatable)
--property-number <key=value>   indexed numeric equality (repeatable)
--property-min <key=value>      indexed numeric lower bound (repeatable)
--property-max <key=value>      indexed numeric upper bound (repeatable)
```

`search-expand` and `context` require `--branch` and exactly one seed selector: `--query`, or one
or more typed filter flags. `--query` cannot be combined with typed filters. Both commands accept:

```text
--branch <name>                 branch-head projection to query (required)
--commit <id>                   current branch-head selector only
--direction out|in|both         edge direction (default: out)
--edge-type <type>              edge type to traverse (repeatable)
--seed-limit <n>                maximum evidence seeds
--max-depth <n>                 maximum traversal depth
--max-visited <n>               maximum visited nodes
--max-rows <n>                  maximum rows
--max-response-bytes <n>        maximum response size
--timeout <duration>            maximum query duration
```

`search` and `filter` support `--continuation <token>` for another page. `--continuation` is also
available on `history`, `branches-containing`, and `diff`; it is not a flag on `search-expand` or
`context`. Retrieval results include the
pinned snapshot, projection provenance (where applicable), effective budgets, and completion or
truncation metadata. Indexed property filters are available only for scalar schema properties with
`indexed = true`.

## History, containment, and diff

```sh
spl history --branch main --entity-id <node-id>
spl history --branch main --commit <reachable-commit-id> --entity-id <node-id> --all-parents
spl branches-containing --entity-id <node-id>
spl branches-containing --snapshot-id <snapshot-id>
spl branches-containing --natural-key <key>
spl diff --base-branch main --target-branch feature
spl diff --base-branch main --base-commit <base-id> \
  --target-branch feature --target-commit <target-id> --max-rows 100
```

`history` requires `--branch` and `--entity-id`; `--commit` must be reachable from that branch.
`--all-parents` traverses every merge parent instead of the first-parent path. `branches-containing`
requires exactly one of `--entity-id`, `--snapshot-id`, and `--natural-key`.

```text
history
  --branch <name>             branch at which to start (required)
  --commit <id>               reachable starting commit
  --entity-id <id>            stable entity ID (required)
  --all-parents               traverse all merge parents
  --continuation <token>
  --max-rows <n> --max-response-bytes <n> --timeout <duration>

branches-containing
  --entity-id <id>            entity selector
  --snapshot-id <id>          exact snapshot selector
  --natural-key <key>         schema-defined natural-key selector
  --continuation <token>
  --max-rows <n> --max-response-bytes <n> --timeout <duration>
```

`diff` requires `--base-branch` and `--target-branch`; each optional commit must be reachable from
its corresponding branch. It supports `--node-id` and `--edge-id` (repeatable filters),
`--node-title-contains`, `--one-hop`, `--continuation`, `--max-rows`, `--max-response-bytes`, and
`--timeout`.

## Contextual reference assets

```sh
spl asset add --branch main --file docs/architecture.md
spl asset add --branch main --file diagram.svg --title "System topology" --id asset-topology
spl asset read --node asset-topology --branch main > diagram.svg
spl asset read --locator spool://assets/<blake3-hash> > artifact.bin
spl asset read <blake3-hash> > artifact.bin
```

`asset add` requires `--branch` and `--file`. It writes the input into local
`.spl/assets/loose` content-addressable storage, computes a BLAKE3-256 hash and MIME type, and
stages an `Asset` node. The node includes `assetUri`, `byteSize`, `mimeType`, and, when available,
`originalFilename`; the default node ID is `asset-<first-16-hash-characters>`. `--title` and
`--id` override the derived title and node ID.

```text
asset add
  --branch <name>  branch on which to stage the asset node (required)
  --file <path>    reference document to ingest (required)
  --title <text>   descriptive Asset node title
  --id <id>        explicit Asset node ID
```

`asset read` accepts exactly one positional locator/node ID, or `--locator`/`--node`. A locator
may be `spool://assets/<64-lowercase-hex-hash>` or a raw BLAKE3 hash. `--branch` selects the
branch when resolving a node ID and otherwise defaults to the active branch. The command streams
raw bytes to stdout rather than emitting JSON. Bound workspaces read from the
context git checkout. Rack on-demand fetch is unsupported for solution context.

```text
asset read [locator-or-node-id]
  --locator <uri-or-hash>  canonical asset URI or raw BLAKE3 hash
  --node <id>              Asset graph node ID
  --branch <name>          branch for node resolution
```

## Merges

Always preserve the preview ID and reuse a caller-owned transaction ID:

```sh
spl merge preview --source feature --target main
spl merge apply --source feature --target main --transaction merge-42 \
  --preview <preview-id> --author alice --message "Merge feature"
spl merge conflicts --target main --transaction merge-42
spl merge resolve --target main --transaction merge-42 --preview <preview-id> \
  --selections selections.json [--overrides mutations.json]
spl merge finalize --target main --transaction merge-42
# or:
spl merge abort --target main --transaction merge-42
```

`preview` requires `--source` and `--target` and does not move refs. `apply` requires
`--source`, `--target`, `--transaction`, and `--preview`; clean previews return a merge commit.
Conflicted previews lease the target branch. `conflicts` inspects the lease, `resolve` requires
`--target`, `--transaction`, `--preview`, and `--selections`, and may take an `--overrides` mutation
array for schema-derived semantic conflicts. `finalize` requires `--target` and `--transaction`;
`abort` requires the same pair and removes the transaction without moving the target.

## Maintenance

```sh
spl fsck
spl gc
spl gc --dry-run
spl gc --repack --grace-period 336h
spl prune --branch feature --dry-run
spl prune --branch feature --author alice --message "Prune transient plan"
spl prune --branch main --force
```

`fsck` is read-only and writes a complete integrity report even when corruption is found; it exits
non-zero for corruption and never repairs data. `gc` retains reachable, reflog, and durable merge
roots, packs retained objects, and prunes only unreachable loose objects after the grace period.
Its flags are `--dry-run`, `--repack`, and `--grace-period` (default `336h`). A committed cleanup
warning still includes the JSON report.

`prune` requires `--branch`, removes nodes marked with the `Ephemeral` modifier label and their
cascading incident edges, and creates a pruning commit when any such nodes are found unless
`--dry-run` is supplied. A zero-match run is an idempotent no-op. `--force` allows pruning the
protected default branch; `--author` and `--message` override commit metadata. It refuses to run
while the branch has staged changes.

## Detached workspaces (deprecated)

`workspace init` / `workspace attach` are **unsupported** for solution context.
Bind N code repos with `.spool/context.toml` instead. Leftover `.spl` manifests
exist only so `spl context export` can migrate once. These commands refuse when
a bind file is present.

## Workspace format migration

```sh
spl migrate --from 1 --to 2
spl workspace migrate --from 1 --to 2
```

`migrate` upgrades leftover `.spl` state-directory format so `spl context export`
can still read it. It is not a dual-run or Rack path.

## Rack remotes (unsupported)

`remote`, `push`, `pull`, and `clone` are **sunset** for solution context. They
are not a parallel SoT, not dual-run, and not pack wire-compat. Do not document
or use a Spool-specific token for context sync. Auth for context is **stock git
credentials only**.

MCP tools for these commands always fail closed. CLI refuses them when
`.spool/context.toml` is present. Context sync is stock `git clone` / fetch /
short-lived branch + PR against the bind remote. Leftover `.spl` graphs move
with `spl context export`.

## MCP Server

```sh
# Start MCP server over standard I/O from a bound code repo
spl mcp
```

`mcp` starts a Model Context Protocol server over stdio using the official SDK
(`github.com/modelcontextprotocol/go-sdk`). Bound graph writes always open a
short-lived branch and pull request. Do not pass `--state-dir` pointing at
`.spl` as a durable context store.

For agents in MCP-equipped environments, native MCP tools are the default
interface; the CLI is the fallback for scripts and CI.

## Version

```sh
spl version
```

`version` prints binary build provenance (release version, commit hash, build date, Go runtime version, and OS/architecture platform) formatted as JSON to standard output without requiring an initialized workspace.

## Completion and help

Generate completion for one of the supported shell subcommands:

```sh
spl completion bash
spl completion zsh
spl completion fish
spl completion powershell
```

`spl help [command path]` prints help for any command, for example `spl help merge apply`.
