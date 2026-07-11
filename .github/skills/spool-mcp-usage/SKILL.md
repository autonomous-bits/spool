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

## Checklist

- [ ] Confirm the tool's auth tier and gather `workspaceId` (+ `stakeholderId` or
      `sessionToken`) before calling.
- [ ] Search or inspect existing chunks/edges first (`spool-search-chunks`,
      `spool-get-neighbourhood`) to avoid duplicating content.
- [ ] Pass `branchId` when the write should be scoped to a draft branch; omit it otherwise.
- [ ] Treat store-surfaced 4xx errors (vocabulary, membership, not-found) as authoritative —
      don't re-guess valid values.
- [ ] Build `apps/mcp` (`pnpm --filter mcp build`) before relying on the local server binary.
