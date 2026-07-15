# onboard-user

Interactive dev/ops helper to onboard a new user into Spool by driving the store's human-only
workspace endpoints (Meridian IDEA-94/IDEA-88/IDEA-95/IDEA-142/IDEA-143): `POST /workspaces`,
`POST /workspaces/:id/members`, and
`POST`/`DELETE /workspaces/:id/stakeholders/:stakeholderId/disciplines`.

This is a plain script, **not** an MCP tool — `apps/store/src/workspaces/workspaces.controller.ts`
deliberately exposes no MCP tool for any of these routes (mirrors the human-only precedent for
branch submit/verify/merge).

## Session token

All four endpoints require a session token for the *acting* stakeholder. There is no headless way
to mint one against the real GitHub OAuth App — a store-issued session token only exists after a
real browser GitHub login (`GET /auth/github/login` / `GET /auth/github/callback`, see
`apps/store/src/auth/auth.controller.ts`).

If `SESSION_TOKEN` isn't set, this script walks you through that login itself: it prints the
login URL to open in your browser (workspace-scoped with `?workspaceId=<id>` for any flow other
than creating a workspace, since those endpoints require a token whose `workspaceId` claim
matches), then prompts you to paste back the `sessionToken` from the callback's JSON response. It
never fabricates, reuses, or falls back to a stub token.

(For scripted/e2e use against the `github-oauth-stub` swapped in by `compose.debug.yaml`, see
`tools/dev-session-token` instead — that stub does not represent a real user.)

## Usage

Requires a running store (default `http://localhost:3000`; use `http://localhost:3002` for the
default `compose.yaml` port mapping).

### Interactive (guided)

```sh
pnpm onboard:user
```

You'll be prompted to choose between:

1. **Create a new workspace** — you become its first member.
2. **Add a user to an existing workspace** — adds an existing stakeholder id as a member of an
   existing workspace id.
3. **Assign a discipline to a workspace member** — grants an existing stakeholder id (already a
   member of the workspace) one of the closed discipline values in that workspace. A stakeholder
   may hold multiple disciplines at once — this is a per-workspace allow-list
   (`stakeholder_disciplines` table), not a single fixed value.
4. **Revoke a discipline from a workspace member** — removes a previously assigned discipline.

Then, unless `SESSION_TOKEN` is already set, you'll be guided through the real GitHub OAuth login
above.

### Non-interactive (scriptable)

```sh
SESSION_TOKEN=<token> pnpm onboard:user -- --create-workspace --name "My Workspace"
SESSION_TOKEN=<token> pnpm onboard:user -- --add-member --workspace-id <id> --stakeholder-id <id>
SESSION_TOKEN=<token> pnpm onboard:user -- --assign-discipline --workspace-id <id> \
  --stakeholder-id <id> --discipline <discipline>
SESSION_TOKEN=<token> pnpm onboard:user -- --revoke-discipline --workspace-id <id> \
  --stakeholder-id <id> --discipline <discipline>
```

Set `SESSION_TOKEN` up front for fully non-interactive/scripted runs; omit it and the script will
still prompt for the pasted-back login token even in flag mode.

### Discipline vocabulary and behavior

`<discipline>` is one of: `product`, `architecture`, `design`, `engineering`, `security`,
`governance` — a closed set validated client-side before calling out, and re-validated by the
store (400 on an invalid value).

- The target stakeholder must already be a member of the workspace, or the store returns 404.
- Assigning is idempotent (`ON CONFLICT DO NOTHING`) — reassigning an already-held discipline
  still succeeds.
- Revoking a discipline that isn't currently assigned is a 404, not a no-op success.
- Like `--add-member`, both discipline flows are workspace-scoped: the session token's
  `workspaceId` claim must match `--workspace-id` (`X-Workspace-Id` header + route param), so
  you'll be prompted to log in scoped to that workspace if `SESSION_TOKEN` isn't already set to a
  matching token.

## Env vars

| Var | Default | Purpose |
|---|---|---|
| `STORE_URL` | `http://localhost:3000` | Base URL of the running store |
| `SESSION_TOKEN` | (unset) | Session token for the acting stakeholder; skips the guided login prompt when set |

Not used by any automated test suite; `tools/` is shared scripts only and application code must
not import from it (see `docs/constitution.md`).
