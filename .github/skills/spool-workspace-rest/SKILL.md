---
name: spool-workspace-rest
description: >
  Creating a Spool store workspace over its human-only REST API, ensuring the acting
  stakeholder ("current user") ends up as a member, and managing per-workspace stakeholder
  discipline allow-lists. Use when an agent needs to call `POST /workspaces`,
  `POST /workspaces/:id/members`, or `POST`/`DELETE /workspaces/:id/stakeholders/:stakeholderId/disciplines`
  directly against the store (as opposed to any `spool-*`/`meridian-*` MCP tool, which don't cover
  these routes).
metadata:
  version: "1.2"
  compatibility: "apps/store REST API, human-only endpoints (Meridian IDEA-94/IDEA-81/IDEA-98/IDEA-142/IDEA-143)"
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

## Managing a stakeholder's discipline allow-list

Since Meridian IDEA-142/IDEA-143, a stakeholder's disciplines are a per-workspace allow-list
(`stakeholder_disciplines` table), not a fixed value baked into the session token. Two more
human-only, workspace-scoped routes manage it:

```sh
# Assign — grants targetStakeholderId the given discipline in this workspace.
curl -X POST http://localhost:3002/workspaces/<workspaceId>/stakeholders/<targetStakeholderId>/disciplines \
  -H "Authorization: ******" \
  -H "X-Workspace-Id: <workspaceId>" \
  -H "Content-Type: application/json" \
  -d '{"discipline": "engineering"}'

# Revoke — removes it again.
curl -X DELETE http://localhost:3002/workspaces/<workspaceId>/stakeholders/<targetStakeholderId>/disciplines/engineering \
  -H "Authorization: ******" \
  -H "X-Workspace-Id: <workspaceId>"
```

- Both routes require `X-Workspace-Id` to match the token's `workspaceId` claim *and* the `:id`
  route param — same `assertWorkspaceScope` check as `/members` (403 on mismatch).
- `targetStakeholderId` must already be a member of the workspace, or the store returns 404
  (checked explicitly, and also on a foreign-key violation from the insert).
- `discipline` is a closed vocabulary (`product`, `architecture`, `design`, `engineering`,
  `security`, `governance`) — an invalid value is a 400, not a silent no-op.
- Assign is idempotent (`ON CONFLICT DO NOTHING`); revoking a discipline that isn't currently
  assigned is a 404, not a no-op success.
- Elsewhere in the API (e.g. `POST /branches/:id/submit`, branch-scoped chunk search/neighbourhood
  queries), the caller supplies a per-request `activeDiscipline` field/query param, which the store
  checks against this same allow-list (400 invalid value, 403 not allowed) — it is no longer
  derived from a fixed discipline on the session token.

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
- [ ] Assign/revoke disciplines only for stakeholders already a member of the workspace; expect
      404 otherwise.
- [ ] Don't rely on a session token's discipline claim for authorization — pass the acting
      discipline as a per-request `activeDiscipline` field/param and expect the store to check it
      against the allow-list, not a fixed token value.
