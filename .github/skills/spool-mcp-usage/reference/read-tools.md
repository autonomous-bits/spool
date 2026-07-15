# Read tools

These `spool-*` tools are read-only against the store. Like the write tools, neither a session
token nor `workspaceId` is a per-call input — both come from the shared
`storeFetch` helper's host-held credentials, the same as every other `spool-*` tool.

| Tool | Store route | Required inputs | Optional inputs |
| --- | --- | --- | --- |
| `spool-search-chunks` | `GET /chunks` | — | `discipline`, `chunkType`, `status`, `contextKind`, `branchId`, `q`, `limit`, `cursor`, `activeDiscipline` |
| `spool-get-neighbourhood` | `GET /chunks/:id/neighbourhood` | `id` | `depth`, `branchId`, `activeDiscipline` |

## Notes

- Use `spool-search-chunks` to discover existing chunks (by full-text `q`, `discipline`,
  `chunkType`, `status`, or `contextKind`) before creating a new one with
  `spool-capture-chunk` — avoid duplicating an existing idea chunk.
- Use `spool-get-neighbourhood` to inspect a chunk's typed edges (both outgoing and incoming)
  before adding a new edge with `spool-create-edge`, so you don't create a redundant or
  conflicting relationship.
- Both tools are safe to call speculatively while exploring a workspace.
- `branchId` scopes results to a draft branch; omit it to see the mainline (approved/promoted)
  graph.
- `activeDiscipline` is **required** whenever `branchId` is supplied (Meridian IDEA-142/IDEA-143):
  the store validates it's a closed-vocabulary value and checks it against the caller's
  per-workspace discipline allow-list, returning 400 (missing/invalid value) or 403 (not allowed
  for that discipline in this workspace) otherwise. It replaces the old fixed, token-baked
  discipline claim — the same stakeholder can hold multiple disciplines per workspace and must
  pick one per call.
