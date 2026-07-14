# onboard-user

Interactive dev/ops helper to onboard a new user into Spool by driving the store's human-only
workspace endpoints (Meridian IDEA-94/IDEA-88/IDEA-95): `POST /workspaces` and
`POST /workspaces/:id/members`.

This is a plain script, **not** an MCP tool — `apps/store/src/workspaces/workspaces.controller.ts`
deliberately exposes no MCP tool for either endpoint (mirrors the human-only precedent for branch
submit/verify/merge).

## Session token

Both endpoints require a session token for the *acting* stakeholder. There is no headless way to
mint one against the real GitHub OAuth App — a store-issued session token only exists after a
real browser GitHub login (`GET /auth/github/login` / `GET /auth/github/callback`, see
`apps/store/src/auth/auth.controller.ts`).

If `SESSION_TOKEN` isn't set, this script walks you through that login itself: it prints the
login URL to open in your browser (workspace-scoped with `?workspaceId=<id>` when adding a
member, since that endpoint requires a token whose `workspaceId` claim matches), then prompts you
to paste back the `sessionToken` from the callback's JSON response. It never fabricates, reuses,
or falls back to a stub token.

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

Then, unless `SESSION_TOKEN` is already set, you'll be guided through the real GitHub OAuth login
above.

### Non-interactive (scriptable)

```sh
SESSION_TOKEN=<token> pnpm onboard:user -- --create-workspace --name "My Workspace"
SESSION_TOKEN=<token> pnpm onboard:user -- --add-member --workspace-id <id> --stakeholder-id <id>
```

Set `SESSION_TOKEN` up front for fully non-interactive/scripted runs; omit it and the script will
still prompt for the pasted-back login token even in flag mode.

## Env vars

| Var | Default | Purpose |
|---|---|---|
| `STORE_URL` | `http://localhost:3000` | Base URL of the running store |
| `SESSION_TOKEN` | (unset) | Session token for the acting stakeholder; skips the guided login prompt when set |

Not used by any automated test suite; `tools/` is shared scripts only and application code must
not import from it (see `docs/constitution.md`).
