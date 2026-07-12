---
name: spool-workspace-rest
description: >
  Creating a Spool store workspace over its human-only REST API and ensuring the acting
  stakeholder ("current user") ends up as a member. Use when an agent needs to call
  `POST /workspaces` or `POST /workspaces/:id/members` directly against the store (as opposed to
  any `spool-*`/`meridian-*` MCP tool, which don't cover workspace creation).
metadata:
  version: "1.1"
  compatibility: "apps/store REST API, human-only endpoints (Meridian IDEA-94/IDEA-81/IDEA-98)"
---

# Spool workspace REST usage

`apps/store` exposes workspace registry endpoints as **human-only REST**, deliberately with no
MCP tool equivalent (`apps/store/src/workspaces/workspaces.controller.ts`). Every call needs a
store-issued session token, not a delegated `stakeholderId` body field.

## Key fact: workspace creation already adds the creator as a member

`WorkspacesService.create` calls `WorkspaceRepository.createWithFirstMember`, which inserts the
workspace **and** a membership row for `claims.stakeholderId` (the token's subject) in one
transaction. `POST /workspaces` alone creates the workspace *and* makes the current user a
member — no separate "add myself" call is needed. Don't follow it with a redundant
`POST /workspaces/:id/members` call for the same stakeholder; it isn't necessary and the token
you just used cannot be used for that route yet (see gotcha below).

## Step-by-step

1. **Obtain a session token for the current user.**
   - Real flow: `GET /auth/github/login` (redirects to GitHub) → `GET /auth/github/callback?code=&state=` →
     returns `{ sessionToken, refreshToken, expiresAt }` (`apps/store/src/auth/auth.controller.ts`).
     If the signed state carries a `cliRedirectUri` (used by the MCP server's own loopback login),
     the callback instead 302-redirects there with a one-time pairing `code`, which must be
     exchanged via `POST /auth/github/pairing/exchange` (`{ code }` → the same token triple) —
     irrelevant for this human-only REST flow, but don't be surprised if you see it in logs.
   - `POST /auth/github/refresh` (`{ refreshToken }` → a new token triple) rotates an
     about-to-expire session token without a full re-login.
   - Local/dev shortcut: `pnpm dev:session-token` from the repo root against a running store
     (`tools/dev-session-token`). It drives the same OAuth flow against the store's stubbed GitHub
     client and prints a ready-to-paste `Authorization: ****** header.
   - Omit `workspaceId` on login to get a **workspace-less bootstrap token** — this is required
     the first time, since the stakeholder has no memberships yet and `POST /workspaces` needs
     exactly that kind of token.

2. **Create the workspace:**
   ```sh
   curl -X POST http://localhost:3002/workspaces \
     -H "Authorization: ******" \
     -H "Content-Type: application/json" \
     -d '{"name": "my-workspace"}'
   ```
   Response: `{ id, name, createdByStakeholderId, createdAt }`. `createdByStakeholderId` is the
   current user (from `claims.stakeholderId`) and is already a member — verify via the response,
   don't assume a second call is required.

3. **Add a *different* stakeholder to the workspace** (only needed for someone other than the
   creator):
   ```sh
   curl -X POST http://localhost:3002/workspaces/<workspaceId>/members \
     -H "Authorization: ******" \
     -H "X-Workspace-Id: <workspaceId>" \
     -H "Content-Type: application/json" \
     -d '{"stakeholderId": "<targetStakeholderId>"}'
   ```
   This route is token-gated *and* workspace-scoped (`assertWorkspaceScope`,
   `apps/store/src/domain/workspace-scope.ts`): `X-Workspace-Id` must equal both the `:id` route
   param and the token's `workspaceId` claim, or the store returns 403.

## Gotcha: your bootstrap token can't call `/members` on the workspace you just created

The token used in step 2 is workspace-less (`workspaceId: null`), so it fails
`assertWorkspaceScope`'s token-tier check for step 3 even against the new workspace. If you need
to add another member right after creating the workspace, re-authenticate first — log in again
with `workspaceId=<new workspace id>` (now valid, since the current user is a member) to mint a
workspace-bound token, then call `POST /workspaces/:id/members` with that token.

## Checklist

- [ ] Never put `createdByStakeholderId`/the acting stakeholder in the `POST /workspaces` body —
      it comes only from the verified session token, never a caller-declared field.
- [ ] Don't call `POST /workspaces/:id/members` to add the creator to their own new workspace —
      it already happened at creation.
- [ ] For adding anyone else, re-authenticate with `workspaceId` set to the new workspace before
      calling `/members`, and send matching `X-Workspace-Id` header + route param.
- [ ] Treat 400 (`Unknown stakeholderId`), 403 (scope violation), and 409 (already a member) as
      authoritative store responses — don't retry blindly.
