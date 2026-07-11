# Session (human-token) tools

These `spool-*` tools sit on the human-only token tier: the store requires a human-authenticated
session token in addition to a workspace id. As of Meridian IDEA-81/G19, neither is a per-call
tool input any more — the MCP process reads `SPOOL_SESSION_TOKEN`/`SPOOL_WORKSPACE_ID` once at
startup (see `apps/mcp/AGENTS.md`) and injects them into every store call via the shared
store-client helper, the same as every other MCP tool. The MCP server still does not issue or
refresh this token itself; it must already be available to the process's environment.

| Tool | Store route | Required inputs | Optional inputs |
| --- | --- | --- | --- |
| `spool-search-chunks` | `GET /chunks` | — | `discipline`, `chunkType`, `status`, `contextKind`, `branchId`, `q`, `limit`, `cursor` |
| `spool-get-neighbourhood` | `GET /chunks/:id/neighbourhood` | `id` | `depth`, `branchId` |

## Notes

- Use `spool-search-chunks` to discover existing chunks (by full-text `q`, `discipline`,
  `chunkType`, `status`, or `contextKind`) before creating a new one with
  `spool-capture-chunk` — avoid duplicating an existing idea chunk.
- Use `spool-get-neighbourhood` to inspect a chunk's typed edges (both outgoing and incoming)
  before adding a new edge with `spool-create-edge`, so you don't create a redundant or
  conflicting relationship.
- Both tools are read-only against the store and safe to call speculatively while exploring a
  workspace.
- `branchId` scopes results to a draft branch; omit it to see the mainline (approved/promoted)
  graph.
