# Session (human-token) tools

These `spool-*` tools sit on the human-only token tier: they require a `sessionToken` obtained
from a prior human authentication with the store, in addition to `workspaceId`. The MCP server
does not issue or refresh this token itself — it must already be available to the caller.

| Tool | Store route | Required inputs | Optional inputs |
| --- | --- | --- | --- |
| `spool-search-chunks` | `GET /chunks` | `sessionToken`, `workspaceId` | `discipline`, `chunkType`, `status`, `contextKind`, `branchId`, `q`, `limit`, `cursor` |
| `spool-get-neighbourhood` | `GET /chunks/:id/neighbourhood` | `id`, `sessionToken`, `workspaceId` | `depth`, `branchId` |

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
