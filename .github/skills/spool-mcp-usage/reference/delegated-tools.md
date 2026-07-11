# Delegated (tokenless) tools

These `spool-*` tools sit on the delegated auth tier (Meridian G11 SG6 / IDEA-92, IDEA-98,
IDEA-100). They never invent a stakeholder identity: every call must forward a caller-supplied
`stakeholderId` and `workspaceId`, and the store enforces that `stakeholderId` already has a
`workspace_memberships` row for that `workspaceId`. No `sessionToken` is needed or accepted.

| Tool | Store route | Required inputs | Optional inputs |
| --- | --- | --- | --- |
| `spool-create-branch` | `POST /branches` | `name`, `discipline`, `stakeholderId`, `workspaceId` | — |
| `spool-capture-chunk` | `POST /chunks` | `label`, `content`, `discipline`, `chunkType`, `contextKind`, `stakeholderId`, `workspaceId` | `branchId` |
| `spool-create-edge` | `POST /edges` | `fromChunkLabel`, `toChunkLabel`, `type`, `discipline`, `stakeholderId`, `workspaceId` | `branchId` |
| `spool-submit-suggestion` | `POST /suggestions` | `discipline`, `stakeholderId`, `workspaceId`, plus **either** chunk-shaped (`label`, `content`) **or** edge-shaped (`fromChunkLabel`, `toChunkLabel`, `relationshipType`) fields — never both | — |
| `spool-submit-verification-signal` | `POST /branches/:branchId/verification-signals` | `branchId`, `verifierName`, `status`, `workspaceId` | `reason` |
| `spool-upload-artifact` | `POST /artifacts` | `content` (base64, ≤700,000 decoded bytes), `mimeType`, `stakeholderId`, `workspaceId` | — |
| `spool-attach-artifact-to-chunk` | `POST /chunks/:label/artifacts` | `chunkLabel`, `artifactId`, `stakeholderId`, `workspaceId` | `branchId` |

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
