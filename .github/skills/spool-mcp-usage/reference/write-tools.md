# Write tools

These `spool-*` tools all write to the store on behalf of the MCP process's authenticated
stakeholder. None of them take `stakeholderId`, `workspaceId`, or a
session token as a per-call input — the shared `storeFetch` helper
(`apps/mcp/src/store-client.ts`) injects the process's `SPOOL_WORKSPACE_ID` and the
GitHub-OAuth-derived session token into every request, and the store derives authorship from the
verified token's `stakeholderId` claim.

| Tool | Store route | Required inputs | Optional inputs |
| --- | --- | --- | --- |
| `spool-create-branch` | `POST /branches` | `name`, `discipline` | — |
| `spool-capture-chunk` | `POST /chunks` | `label`, `content`, `discipline`, `chunkType`, `contextKind` | `branchId` |
| `spool-create-edge` | `POST /edges` | `fromChunkLabel`, `toChunkLabel`, `type`, `discipline` | `branchId` |
| `spool-submit-suggestion` | `POST /suggestions` | `discipline`, plus **either** chunk-shaped (`label`, `content`) **or** edge-shaped (`fromChunkLabel`, `toChunkLabel`, `relationshipType`) fields — never both | — |
| `spool-submit-verification-signal` | `POST /branches/:branchId/verification-signals` | `branchId`, `verifierName`, `status` | `reason` |
| `spool-upload-artifact` | `POST /artifacts` | `content` (base64, ≤700,000 decoded bytes), `mimeType` | — |
| `spool-attach-artifact-to-chunk` | `POST /chunks/:label/artifacts` | `chunkLabel`, `artifactId` | `branchId` |

## Notes

- `discipline`, `chunkType`, `contextKind`, `type`/`relationshipType`, and `status` are closed
  vocabularies enforced by the store, not by the MCP tool. Expect the store's own 4xx message on
  an invalid value — do not guess the vocabulary ahead of time; ask the store (or a human) if
  unsure.
- `branchId` is optional everywhere it appears. Omit it for the branchless/mainline path; supply
  it to scope the write to a specific draft branch.
- `spool-submit-suggestion` never accepts or rejects a suggestion — suggestion decisions are
  human-only (Meridian IDEA-75). Use it only to submit a new chunk/edge suggestion for later
  review.
- `spool-upload-artifact` rejects malformed base64 or content decoding to more than 700,000 bytes
  before ever calling the store.
- If a call 403s, it's almost always because the token's stakeholder isn't a member of the
  process's `SPOOL_WORKSPACE_ID` workspace — not a missing per-call argument.
