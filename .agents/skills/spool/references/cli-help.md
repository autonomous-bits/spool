# CLI help

`spl` is a JSON-oriented CLI: successful command results are written to stdout. The executable
logs failures as structured JSON on stderr and exits non-zero. Help itself is human-readable.

Inspect the installed command surface instead of assuming a release still has a removed command:

```sh
spl --help
spl <command> --help
spl merge --help
```

`spl --help` lists the KEEP surface only. Context-management commands require
`.spool/context.toml` (`solution_id`, `remote`, `protected_branch`) and refuse unbound workspaces.

## Command index

| Command | Purpose |
| --- | --- |
| `context init` | Bind this code repo to a context git remote |
| `context export`, `context migrate-once` | Export leftover `.spl` into bound context git |
| `query-context` | Evidence-focused bounded graph context |
| `search`, `filter`, `search-expand`, `resolve`, `graph` | Query the bound checkout |
| `mutate` | Write node/edge mutations (one batch → branch+PR) |
| `schema migrate`, `validate` | Write a schema (branch+PR) and validate |
| `merge preview/apply/conflicts/resolve/finalize/abort` | File-graph merge |
| `prune` | Remove `Ephemeral` nodes and cascading edges |
| `asset add/read` | Store and stream reference assets |
| `mcp` | KEEP MCP server over stdio |
| `version` | Print build provenance as JSON |
| `completion`, `help` | Shell completion and command help |

### Removed (deleted; no aliases)

`init`, `workspace *`, `remote *`, `push`, `pull`, `clone`, old `migrate`, `fsck`, `gc`, pack prune,
`cherry-pick`, `add`, `status`, `commit`, `branch`, `switch`, `history`, `diff`,
`branches-containing`. Use stock git for history and diff.

All commands accept `-h, --help`.

## Bind and leftover export

```sh
spl context init --remote https://github.com/org/solution-context.git
spl context export
spl context migrate-once
```

```text
context init
  --remote <url>             context git remote (required)
  --solution-id <id>         defaults to the current directory name
  --protected-branch <name>  defaults to main
  --repository-id <id>       code-repo namespace for node IDs
  --author <text>            git author for the layout commit

context export / migrate-once
  --from <path>              leftover .spl state directory
  --branch <name>            leftover .spl branch to export
  --author <text>
  --message <text>
```

The `context` namespace is **not** the query verb. Use `query-context`.

## Mutate

```sh
spl mutate --operations mutations.json --message "Record requirement"
```

```text
mutate
  --operations <path|->  JSON mutation-operation array (required; - reads stdin)
  --author <text>
  --message <text>
```

`mutate` is bound-only and refuses unbound workspaces. One ops batch becomes one git commit on a
short-lived branch plus pull request. Pass `-` to `--operations` to read stdin. Identical or empty
effective diffs do not open a PR. Use `schema migrate` when changing `schema.toml`. There are no
aliases to `add` / `commit` / `status` / `stage` / `write`.

## Reading graphs

```sh
spl resolve --node <node-id>
spl graph
spl search --query incident
spl filter --label Task --property-text status=open
spl search-expand --query incident --direction out --edge-type RELATES_TO
spl query-context --label Task --property-text status=open --direction both
```

```text
resolve
  --node <id>                 stable node entity ID (required)

search
  --query <text>              lexical query (required)

filter
  --label <label>             required node label (repeatable)
  --property-text <key=value>
  --property-number <key=value>
  --property-min <key=value>
  --property-max <key=value>

search-expand / query-context
  --query <text>              exclusive with typed filters
  --label / property-*        typed seed selector
  --direction out|in|both     default: out
  --edge-type <type>          repeatable
  --seed-limit <n>
```

## Schemas

```sh
spl schema migrate --schema people.toml --batch people-mutations.json
spl validate
```

```text
schema migrate
  --schema <path>  target TOML schema (required)
  --batch <path>   optional JSON mutation-operation array
  --author <text>
  --message <text>

validate
  (no flags; bound checkout)
```

Identical schema content is a no-op (no empty PR).

## Assets

```sh
spl asset add --file docs/architecture.md --title "Architecture notes"
spl asset read --node notes > architecture.md
```

```text
asset add
  --file <path>    required
  --title <text>
  --id <id>
  --author <text>
  --message <text>

asset read [locator-or-node-id]
  --locator <uri-or-hash>
  --node <id>
```

`asset add` opens a short-lived branch + PR.

## Merges

File-graph merge of two git branches. Not a Rack/.spl lease.

```sh
spl merge preview --source feature --target main
spl merge apply --source feature --target main --transaction merge-42 \
  --preview <preview-id> --author alice --message "Merge feature"
spl merge conflicts --transaction merge-42
spl merge resolve --transaction merge-42 --preview <preview-id> \
  --selections selections.json [--overrides mutations.json]
spl merge finalize --transaction merge-42
spl merge abort --transaction merge-42
```

## Prune

```sh
spl prune --dry-run
spl prune --author alice --message "Prune transient plan"
```

Graph cleanup of `Ephemeral` nodes and cascading edges on the bound checkout. Writes a
short-lived branch + PR. Unbound workspaces are refused. This is not pack/CAS GC.

## MCP, version, completion

```sh
spl mcp
spl version
spl completion bash
spl help query-context
```

`mcp` exposes the KEEP tool set (20 tools), matching `spl --help`.
