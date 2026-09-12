---
name: spool
description: Use Spool graph version control via native MCP tools (`spl_*`) by default, falling back to the local CLI (`spl`) if MCP is unavailable. Initialize repositories, stage and commit graph changes, manage branches and merges, query graph snapshots, validate schemas, and maintain repository integrity.
---

# Spool

Spool is a local, content-addressed graph version-control system.

## Interaction Protocol: MCP Default with CLI Fallback

AI coding assistants and agents should interact with Spool using the following rule of precedence:

1. **DEFAULT — Native MCP Tools (`spl_*`)**:
   - If the Spool MCP server is running and its tools (`spl_*`) are available in your toolset, **always prefer using the MCP tools as the primary interface**.
   - **Why**: MCP tools use structured, typed JSON schemas, eliminate disk I/O for staging (`spl_add` takes mutations directly as an in-memory JSON array without creating temporary files), handle concurrent repository locking safely on the server side, provide structured conflict inspection, and preserve rich error envelopes with warnings.

2. **FALLBACK — Local CLI (`spl <command>`)**:
   - Fall back to executing CLI commands (`spl <command>`) via shell tools **only if**:
     - The Spool MCP server or tools are not configured or available in your execution environment.
     - An MCP tool call fails due to a transport, connection, or protocol failure.
     - You are running automated shell scripts or CI/CD pipelines directly in bash/zsh.
   - When using the CLI, all successful commands emit JSON to stdout, and errors are written to stderr.

---

## Configuring the Spool MCP Server

Spool includes a native Model Context Protocol (MCP) server built with the official Go SDK (`github.com/modelcontextprotocol/go-sdk`). It runs over standard I/O via `spl mcp`.

### Prerequisites
- Build or install the `spl` binary:
  ```sh
  go build -o dist/spl ./cmd/spl
  # Or install to your GOPATH/bin:
  go install ./cmd/spl
  ```
- Ensure `spl` is located in your system `$PATH` (or use the absolute path to the binary).

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

To bind to a specific Spool state directory explicitly:
```json
{
  "mcpServers": {
    "spool": {
      "command": "spl",
      "args": ["mcp", "--state-dir", "/absolute/path/to/repo/.spl"]
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
Add an MCP server entry pointing to `spl mcp` in your global or workspace configuration:
```json
{
  "command": "spl",
  "args": ["mcp"]
}
```

---

## MCP Tool to CLI Mapping

| Category | Primary MCP Tool | CLI Fallback | Key Parameters & Notes |
| :--- | :--- | :--- | :--- |
| **Staging** | `spl_status` | `spl status --branch <b\>` | Inspect staged mutations for branch. |
| **Staging** | `spl_add` | `spl add --branch <b\> --batch <f\>` | **MCP Advantage**: accepts `mutations` JSON array directly in-memory; no disk file needed. |
| **Commits** | `spl_commit` | `spl commit --branch <b\> --author <a\> --message <m\>` | Creates an immutable commit from staged changes. |
| **Branches** | `spl_branch_list` | `spl branch list` | Lists all local branches and marks active HEAD. |
| **Branches** | `spl_branch_create` | `spl branch create <n\> --from-branch <b\>` | Creates a branch from existing branch or commit. |
| **Branches** | `spl_branch_delete` | `spl branch delete <n\>` | Deletes an inactive branch. |
| **Branches** | `spl_switch` | `spl switch <branch\>` | Switches the active HEAD pointer. |
| **Containment**| `spl_branches_containing` | `spl branches-containing --entity-id/--natural-key` | Finds branches containing an entity, snapshot, or natural key. Supports `budget`. |
| **Reads** | `spl_resolve` | `spl resolve --branch <b\> --node <id\>` | Fetches node by stable ID. Supports `commit` and `budget`. |
| **Reads** | `spl_search` | `spl search --branch <b\> --query <q\>` | Full-text search (FTS5). Supports `commit`, `continuation`, and `budget`. |
| **Reads** | `spl_filter` | `spl filter --branch <b\> --label <l\>` | Filters by labels and typed `predicates`. Supports `commit` and `budget`. |
| **Reads** | `spl_context` | `spl context --branch <b\> --query <q\>` | Bounded context expansion. Supports query/labels/predicates, direction, edge types, `budget`. |
| **Reads** | `spl_search_expand` | `spl search-expand --branch <b\> ...` | Seed retrieval + graph traversal. Supports query/labels/predicates, direction, edge types, `budget`. |
| **Diff** | `spl_diff` | `spl diff --base-branch <bb\> --target-branch <tb\>` | Graph diff. Supports `node_ids`, `edge_ids`, `node_title_contains`, `one_hop`, `continuation`, `budget`. |
| **History** | `spl_history` | `spl history --branch <b\> --node <id\>` | Entity commit log. Supports `commit`, `all_parents`, `continuation`, `budget`. |
| **Graph** | `spl_graph` | `spl graph --branch <b\>` | Full graph dump of branch snapshot. Supports `commit`. |
| **Merge** | `spl_merge_preview` | `spl merge preview --source <s\> --target <t\>` | Computes 3-way graph preview and preview ID. |
| **Merge** | `spl_merge_apply` | `spl merge apply --source <s\> --target <t\> ...` | Applies preview. Exposes structured conflicts on conflict error. |
| **Merge** | `spl_merge_conflicts` | `spl merge conflicts --target <t\> --transaction <tx\>` | Inspects persisted conflict state. |
| **Merge** | `spl_merge_resolve` | `spl merge resolve ... --selections <sel\>` | Records conflict choices (`source`/`target`) and overrides. |
| **Merge** | `spl_merge_finalize` | `spl merge finalize --target <t\> --transaction <tx\>` | Finalizes resolved merge into a merge commit. |
| **Merge** | `spl_merge_abort` | `spl merge abort --target <t\> --transaction <tx\>` | Aborts active merge transaction. |
| **Lifecycle** | `spl_init` | `spl init` | Initializes repository state directory. |
| **Lifecycle** | `spl_fsck` | `spl fsck` | Validates repository graph and storage integrity. |
| **Lifecycle** | `spl_gc` | `spl gc` | Garbage collects unreferenced objects. |
| **Lifecycle** | `spl_prune` | `spl prune --branch <b\>` | Excises `Ephemeral` labeled nodes and incident edges. |
| **Lifecycle** | `spl_cherry_pick` | `spl cherry-pick --commit <c\> --target-branch <b\>` | Selectively transplants a commit. |
| **Schema** | `spl_schema_migrate` | `spl schema migrate --branch <b\> --schema <f\>` | Stages schema migration (accepts file path or inline TOML). |
| **Schema** | `spl_validate` | `spl validate --branch <b\>` | Validates graph against schema invariants. |
| **Assets** | `spl_asset_add` | `spl asset add --branch <b\> --file <p\>` | Content-addresses and stores reference asset blob. |
| **Assets** | `spl_asset_read` | `spl asset read --branch <b\> --node <id\>` | Reads asset bytes (Base64-encoded via MCP, raw on CLI). |
| **Workspace** | `spl_workspace_init` | `spl workspace init <name\>` | Initializes detached multi-repo workspace. |
| **Workspace** | `spl_workspace_attach` | `spl workspace attach --workspace <w\> ...` | Binds directory to workspace catalog. |
| **Workspace** | `spl_migrate` | `spl migrate --from <v\> --to <v\>` | Migrates repository state format. |
| **Remote** | `spl_remote_set` | `spl remote set --endpoint <url\> ...` | Configures Rack remote endpoint and auth. |
| **Remote** | `spl_remote_show` | `spl remote show` | Probes Rack remote connectivity and versions. |
| **Remote** | `spl_remote_remove` | `spl remote remove` | Clears remote configuration. |
| **Remote** | `spl_remote_branch` | `spl remote branch <action\> ...` | Manages remote branch references on Rack. |
| **Remote** | `spl_push` | `spl push --branch <b\>` | Pushes commits to Rack. Supports `reconcile: true`. |
| **Remote** | `spl_pull` | `spl pull --branch <b\>` | Pulls commits and fast-forwards local branch. |
| **Remote** | `spl_clone` | `spl clone <url\> [dir]` | Clones remote repository into new directory. |
| **Metadata** | `spl_version` | `spl version` | Inspects binary version, commit, build date. |

---

## Branch Strategy & User Elicitation

Before staging or committing changes:
1. **Check active branch**:
   - **MCP**: Call `spl_branch_list`.
   - **CLI**: Run `spl branch list`.
2. **Elicit user intent**: Unless the user has explicitly requested a target branch, prompt the user to clarify whether changes should be:
   - Committed directly to the active branch (e.g. `main`), or
   - Isolated on a new dedicated branch to allow review, diffing, and isolated merging.
3. **Execute branch setup**:
   - If a new branch was requested:
     - **MCP**: Call `spl_branch_create(name, from_branch)`, then `spl_switch(name)`.
     - **CLI**: Run `spl branch create <name> --from-branch <current>` followed by `spl switch <name>`.

Before merging a branch containing transient planning data, preview its cleanup:
- **MCP**: Call `spl_prune(branch, dry_run: true)`. If clean, call `spl_prune(branch, author, message)`.
- **CLI**: Run `spl prune --branch <branch> --dry-run`, followed by `spl prune --branch <branch> --author ... --message ...`.

---

## Common Invocation Rules & Pitfalls

1. **Native Queries Only**: Always use native Spool query tools (`spl_filter`, `spl_search`, `spl_resolve`, `spl_context`, `spl_diff`, `spl_graph`) or their CLI counterparts. Never pipe JSON outputs to Python, jq, awk, or ad-hoc shell parsing scripts.
2. **`resolve` / `spl_resolve`**: Requires `node` (node entity ID).
3. **`diff` / `spl_diff`**: Uses `base_branch` and `target_branch` (do not use `--from` or `--to`).
4. **`merge_apply` / `spl_merge_apply`**: Requires `preview_id` (obtained from `spl_merge_preview`), `transaction_id`, `source`, `target`, `author`, and `message`.
5. **`context` & `search_expand`**:
   - Do **NOT** pass `--id` or `id`.
   - Use `query` OR `label`/`labels`/`predicates` (mutually exclusive with `query`).
   - Use `direction`: `"out"` (default), `"in"`, or `"both"`.
   - Use `max_depth` (not `depth`), `seed_limit`, and `edge_types`.
   - To inspect a single specific node by ID, use `spl_resolve` (`spl resolve`).

---

## Command Reference Index

| Commands | Reference |
| :--- | :--- |
| `init`, `add`, `status`, `commit` | [Working changes](references/working-changes.md) |
| Authoring `add` batches and atomic ideas | [Batch authoring](references/batch-authoring.md) |
| `branch`, `switch`, `history`, `branches-containing`, `diff`, `cherry-pick` | [Branches and history](references/branches-and-history.md) |
| `schema migrate`, `validate` | [Schemas](references/schemas.md) |
| `resolve`, `search`, `filter`, `search-expand`, `context` | [Reading graphs](references/reading-graphs.md) |
| `merge` cycle (`preview`, `apply`, `conflicts`, `resolve`, `finalize`, `abort`) | [Merges](references/merges.md) |
| `fsck`, `gc`, `prune` | [Maintenance](references/maintenance.md) |
| `asset add`, `asset read` | [CLI help](references/cli-help.md) |
| `workspace init`, `workspace attach`, `migrate` | [Multi-repo workspaces](references/workspaces.md) |
| `remote`, `push`, `pull`, `clone`, `mcp` | [CLI help](references/cli-help.md) |
| `version`, `completion`, `help` | [CLI help](references/cli-help.md) |
