---
name: spool-mcp-usage
description: >
  Effective use of the local `spool` MCP server's tools for capturing chunks, creating
  branches/edges, submitting suggestions, uploading artifacts, and verifying branches. Use
  when an agent needs to call any `spool-*` MCP tool.
metadata:
  version: "2.0"
  compatibility: "apps/mcp spool MCP server (stdio), unified process-level auth"
---

# Spool MCP usage

All 9 `spool-*` tools authenticate identically. None of them accept `stakeholderId`,
`workspaceId`, or `sessionToken` as a per-call input; every call goes through the MCP process's
shared
`storeFetch` helper (`apps/mcp/src/store-client.ts`), which injects `Authorization: Bearer
<sessionToken>` and `X-Workspace-Id` on every request. Pick the right reference:

- [Write tools](./reference/write-tools.md) — `spool-create-branch`, `spool-capture-chunk`,
  `spool-create-edge`, `spool-submit-suggestion`, `spool-submit-verification-signal`,
  `spool-upload-artifact`, `spool-attach-artifact-to-chunk`.
- [Read tools](./reference/read-tools.md) — `spool-search-chunks`, `spool-get-neighbourhood`.
- [Common workflows](./reference/workflows.md) — end-to-end tool sequences for capturing
  chunks, relating chunks, submitting suggestions, attaching artifacts, and recording
  verification signals.

## Prerequisite: process-level auth, not per-call auth

The MCP process needs, at startup:

- `SPOOL_WORKSPACE_ID` (required) — the workspace every tool call is scoped to for this process's
  lifetime. There is no way to target a different workspace from a single running MCP server; a
  different workspace means a different server config (see `.mcp.json`/`.vscode/mcp.json`).
- `SPOOL_SESSION_TOKEN` (optional override) — set only for headless/CI contexts. When present, it
  is used verbatim on every call and the interactive login below is skipped entirely.

Without an override, the server authenticates via GitHub OAuth (`apps/mcp/src/auth/login-flow.ts`):
on first use (or once cached credentials expire and can't be refreshed) it opens a browser to the
store's `/auth/github/login`, waits on a local loopback callback, exchanges the resulting pairing
code at `/auth/github/pairing/exchange`, and caches the resulting session/refresh tokens in the OS
keyring (`apps/mcp/src/auth/token-cache.ts`, service `spool-mcp`, keyed by store URL + workspace
id). Subsequent calls reuse or silently refresh that cached token — no repeated browser prompts.

The workspace itself (and the calling stakeholder's membership in it) must already exist — no
`spool-*` tool creates either. Use the **meridian** MCP server's `create_workspace` and
`add_workspace_member` tools, or the store's human-only workspace REST API (see the
`spool-workspace-rest` skill), or confirm the IDs with a human first. A 403 from any `spool-*`
call most often means the token's stakeholder isn't a member of `SPOOL_WORKSPACE_ID`.

## Local context cache (branch tracking only)

The repo root keeps a gitignored `.spool/context.json` cache for **branch bookkeeping only**
(`workspaceId`/`stakeholderId` are process-level env config, not per-call/per-agent state, so
they don't belong in this file):

```json
{
  "currentBranchId": "... or null",
  "availableBranches": [{ "id": "...", "name": "...", "discipline": "...", "status": "..." }],
  "updatedAt": "..."
}
```

- Read it before scoping a new chunk/edge write to a branch, so you reuse an existing draft
  branch instead of creating a redundant one.
- After `spool-create-branch` succeeds, or after deliberately switching branches, update
  `currentBranchId` and refresh `availableBranches` in the file immediately — don't let it go
  stale.
- This file is local scratch state (gitignored via `.spool/`), not committed content — never rely
  on it being present in a fresh clone or CI, and never put secrets (tokens) in it; tokens live
  only in the OS keyring via the token cache, never in this file.

## Checklist

- [ ] Confirm `SPOOL_WORKSPACE_ID` is set for the running MCP process (check `.mcp.json`); don't
      look for a per-call `workspaceId`/`stakeholderId` argument — none of the tools take one.
- [ ] If a call fails auth, don't ask for a token — either it's `SPOOL_SESSION_TOKEN` (env,
      headless override) or the interactive GitHub login handles it; a 403 is a membership
      problem, not a missing-credential problem.
- [ ] Search or inspect existing chunks/edges first (`spool-search-chunks`,
      `spool-get-neighbourhood`) to avoid duplicating content.
- [ ] Default to scoping new writes to a draft branch: use `currentBranchId` from
      `.spool/context.json`, or create one with `spool-create-branch` first, unless a
      branchless/mainline write is explicitly requested. Update the context cache with the
      branch you used.
- [ ] Treat store-surfaced 4xx errors (vocabulary, membership, not-found) as authoritative —
      don't re-guess valid values.
- [ ] Build `apps/mcp` (`pnpm --filter mcp build`) before relying on the local server binary.
