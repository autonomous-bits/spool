---
name: spool-mcp-usage
description: >
  Effective use of the local `spool` MCP server's tools for capturing chunks, creating
  branches/edges, submitting suggestions, uploading artifacts, and verifying branches. Use
  when an agent needs to call any `spool-*` MCP tool.
metadata:
  version: "1.0"
  compatibility: "apps/mcp spool MCP server (stdio), local store at SPOOL_STORE_URL"
---

# Spool MCP usage

The `spool` MCP server (`apps/mcp`) exposes tools split across two auth tiers. Pick the right
reference before calling a tool:

- [Delegated (tokenless) tools](./reference/delegated-tools.md) — `spool-create-branch`,
  `spool-capture-chunk`, `spool-create-edge`, `spool-submit-suggestion`,
  `spool-submit-verification-signal`, `spool-upload-artifact`,
  `spool-attach-artifact-to-chunk`. Require `stakeholderId` + `workspaceId`.
- [Session (human-token) tools](./reference/session-tools.md) — `spool-search-chunks`,
  `spool-get-neighbourhood`. Require a pre-obtained `sessionToken` + `workspaceId`.
- [Common workflows](./reference/workflows.md) — end-to-end tool sequences for capturing
  chunks, relating chunks, submitting suggestions, attaching artifacts, and recording
  verification signals.

## Prerequisite

The store validates every call against a `workspace_memberships` row for `workspaceId` +
`stakeholderId` (or the token's `workspaceId` claim). The workspace and the stakeholder's
membership must already exist — no `spool-*` tool creates them. Use the **meridian** MCP
server's `create_workspace`, `register_stakeholder`, and `add_workspace_member` tools first, or
confirm the IDs with a human, or every `spool-*` call fails with a 403.

## Local context cache (load before every `spool-*` call)

The repo root keeps a gitignored `.spool/context.json` cache with the IDs needed for delegated
calls:

```json
{
  "workspaceId": "...",
  "workspaceName": "...",
  "stakeholderId": "...",
  "stakeholderName": "...",
  "currentBranchId": "... or null",
  "availableBranches": [{ "id": "...", "name": "...", "discipline": "...", "status": "..." }],
  "updatedAt": "..."
}
```

- **Always read `.spool/context.json` before making any `spool-*` MCP call.** Never guess or
  re-derive `workspaceId`/`stakeholderId` from conversation history alone.
- If the file is missing, empty, or its `workspaceId`/`stakeholderId` can't be confirmed still
  valid, resolve them (ask the human, or query the store/meridian) before proceeding, then write
  the resolved values back to the file.
- After `spool-create-branch` succeeds, or after deliberately switching branches, update
  `currentBranchId` and refresh `availableBranches` in the file immediately — don't let it go
  stale.
- This file is local scratch state (gitignored via `.spool/`), not committed content — never rely
  on it being present in a fresh clone or CI, and never put secrets in it.

## Checklist

- [ ] Load `.spool/context.json` first; resolve and persist `workspaceId`/`stakeholderId` if
      missing or stale.
- [ ] Confirm the tool's auth tier and gather `workspaceId` (+ `stakeholderId` or
      `sessionToken`) before calling.
- [ ] Search or inspect existing chunks/edges first (`spool-search-chunks`,
      `spool-get-neighbourhood`) to avoid duplicating content.
- [ ] Default to scoping new writes to a draft branch: use `currentBranchId` from the context
      cache, or create one with `spool-create-branch` first, unless a branchless/mainline write
      is explicitly requested. Update the context cache with the branch you used.
- [ ] Treat store-surfaced 4xx errors (vocabulary, membership, not-found) as authoritative —
      don't re-guess valid values.
- [ ] Build `apps/mcp` (`pnpm --filter mcp build`) before relying on the local server binary.
