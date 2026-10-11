---
name: spool
description: Use Spool graph version control via native MCP tools (`spl_*`) by default, falling back to the local CLI (`spl`) if MCP is unavailable. Bind a code repo to context git, query the bound checkout, migrate schemas, manage assets and file-graph merges, and prune ephemeral nodes.
---

# Spool

Spool stores solution context as human-diffable graph files in a git remote. Git is the source of truth. Bind a code repository with `.spool/context.toml`, then query and mutate through KEEP CLI/MCP tools only. Mutations open a short-lived branch and pull request.

Call only commands and tools advertised by `spl --help` and MCP `tools/list`. If a name is absent, it does not exist.

## Interaction Protocol: MCP Default with CLI Fallback

1. **DEFAULT — Native MCP Tools (`spl_*`)**:
   - If the Spool MCP server is running, prefer its KEEP tools.
   - Tools are bound-only. Checkouts without `.spool/context.toml` fail closed.

2. **FALLBACK — Local CLI (`spl <command>`)**:
   - Use the CLI when MCP is unavailable, in shell scripts, or in CI.
   - Successful commands emit JSON to stdout; errors go to stderr.

---

## Configuring the Spool MCP Server

Spool includes a native MCP server via `spl mcp`.

### Prerequisites
- Build or install the `spl` binary:
  ```sh
  go build -o dist/spl ./cmd/spl
  go install ./cmd/spl
  ```
- Ensure `spl` is on `$PATH`.

### Client Configuration Examples

#### 1. Claude Desktop / Claude Code (`claude_desktop_config.json`)
```json
{
  "mcpServers": {
    "spool": {
      "command": "spl",
      "args": ["mcp"]
    }
  }
}
```

#### 2. VS Code / Cursor (`mcp.json` or workspace settings)
```json
{
  "mcpServers": {
    "spool": {
      "command": "spl",
      "args": ["mcp"]
    }
  }
}
```

#### 3. Google Antigravity / Gemini CLI
```json
{
  "command": "spl",
  "args": ["mcp"]
}
```

---

## MCP Tool to CLI Mapping (KEEP only)

| Category | Primary MCP Tool | CLI Fallback | Notes |
| :--- | :--- | :--- | :--- |
| **Bind** | *(CLI only)* | `spl context init --remote <url>` | Writes `.spool/context.toml`. |
| **Export** | `spl_context_export` | `spl context export` / `migrate-once` | Escape hatch only. See below. |
| **Reads** | `spl_resolve` | `spl resolve --node <id>` | Bound checkout. |
| **Reads** | `spl_search` | `spl search --query <q>` | Lexical FTS on the projection. |
| **Reads** | `spl_filter` | `spl filter --label <l>` | Labels and typed predicates. |
| **Reads** | `spl_query_context` | `spl query-context --query <q>` | Bounded graph context. |
| **Reads** | `spl_search_expand` | `spl search-expand --query <q>` | Seed retrieval + graph traversal. |
| **Graph** | `spl_graph` | `spl graph` | Full bound snapshot. |
| **Mutate** | `spl_mutate` | `spl mutate --operations <f>` | One node/edge ops batch → short-lived branch + PR. Bound-only. |
| **Merge** | `spl_merge_preview` | `spl merge preview --source <s> --target <t>` | File-graph three-way preview. |
| **Merge** | `spl_merge_apply` | `spl merge apply ...` | Clean apply → short-lived branch + PR. |
| **Merge** | `spl_merge_conflicts` | `spl merge conflicts --transaction <tx>` | Cache-backed conflict state. |
| **Merge** | `spl_merge_resolve` | `spl merge resolve ... --selections <sel>` | Source/target choices + overrides. |
| **Merge** | `spl_merge_finalize` | `spl merge finalize --transaction <tx>` | Resolved merge via branch + PR. |
| **Merge** | `spl_merge_abort` | `spl merge abort --transaction <tx>` | Discard conflicted merge. |
| **Schema** | `spl_schema_migrate` | `spl schema migrate --schema <f>` | Schema + optional mutation batch → PR. |
| **Schema** | `spl_validate` | `spl validate` | Bound schema.toml check. |
| **Assets** | `spl_asset_add` | `spl asset add --file <p>` | Ingest + commit via branch + PR. |
| **Assets** | `spl_asset_read` | `spl asset read --node <id>` | Stream bytes (Base64 via MCP). |
| **Prune** | `spl_prune` | `spl prune` | Ephemeral nodes/edges. |
| **Metadata** | `spl_version` | `spl version` | Binary provenance. |

If you still have a leftover `.spl` store, run `context export` / `migrate-once` once into the bound context git remote, then stop using `.spl`. Export is lossy; re-run is overwrite-at-own-risk.

History, diff, and pull-request review of the context remote use **stock git** and GitHub (`git log`, `git diff`, the host pull request).

---

## Bound checkout and writes

Context-management KEEP tools require `.spool/context.toml`. If the checkout is unbound, fail closed and tell the user to run `spl context init --remote`.

Writes never push the protected branch. They open `spool/mcp/<stamp>-<nonce>` and a host PR. Identical schema or mutate diffs are a no-op (no empty PR).

Routine node/edge writes (same `MutationOperation` shape as schema migrate):

- **MCP**: `spl_mutate(operations: [...], author, message)`.
- **CLI**: `spl mutate --operations mutations.json --author ... --message ...` (`--operations -` reads stdin).

Use `schema migrate` only when changing `schema.toml`.

Before pruning ephemeral planning data:

- **MCP**: `spl_prune(dry_run: true)`, then `spl_prune(author, message)`.
- **CLI**: `spl prune --dry-run`, then `spl prune --author ... --message ...`.

---

## Common Invocation Rules & Pitfalls

1. **Native Queries Only**: Use `spl_filter`, `spl_search`, `spl_resolve`, `spl_query_context`, `spl_graph` (or CLI). Never pipe JSON through Python/jq for graph queries.
2. **`resolve` / `spl_resolve`**: Requires `node`.
3. **`query-context` and `search-expand`**:
   - Do **NOT** pass `--id` or `id`.
   - Use `query` **OR** `label`/`labels`/`predicates` (mutually exclusive with `query`).
   - Use `direction`: `"out"` (default), `"in"`, or `"both"`.
   - Use `seed_limit` and `edge_types`.
   - The `context` CLI namespace is **not** the query verb. It is only `init`, `export`, and `migrate-once`.
4. History, diff, and pull-request review: stock git and GitHub on the context remote.

---

## Command Reference Index

| Commands | Reference |
| :--- | :--- |
| `context init`, one-shot `context export` / `migrate-once` | [Working changes](references/working-changes.md) |
| Authoring mutation batches for `mutate` | [Batch authoring](references/batch-authoring.md) |
| History and diff (stock git); file-graph `merge` | [Branches and history](references/branches-and-history.md) |
| `mutate`, `schema migrate`, `validate` | [Schemas](references/schemas.md) |
| `resolve`, `search`, `filter`, `search-expand`, `query-context` | [Reading graphs](references/reading-graphs.md) |
| `merge` cycle | [Merges](references/merges.md) |
| `prune` | [Maintenance](references/maintenance.md) |
| `asset add`, `asset read`, `mcp`, `version` | [CLI help](references/cli-help.md) |
| Bind and one-shot export | [Bind](references/workspaces.md) |
