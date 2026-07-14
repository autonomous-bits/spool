---
name: spool-local-dev-auth
description: >
  Obtaining a valid apps/store session token for local development by driving the real GitHub
  OAuth login flow and asking the user to paste back the resulting token. Use when an agent needs
  a `sessionToken` to call human-only store REST endpoints (e.g. `POST /workspaces`,
  `POST /workspaces/:id/members`) or any other Authorization-gated store route, and no cached
  token is already available.
metadata:
  version: "1.0"
  compatibility: "apps/store real GitHub OAuth flow (compose.yaml), Meridian IDEA-81/IDEA-98/IDEA-101"
---

# Spool local-dev session token

`apps/store`'s human-only REST endpoints (workspace creation/membership, and any other
`Authorization`-gated route) require a store-issued session token minted through a real GitHub
OAuth login — there is no MCP tool or headless shortcut that produces one against the real
GitHub OAuth App wired in `compose.yaml`.

This skill drives that flow interactively: it gives the user a login URL to open in their
browser, then asks them to paste back the `sessionToken` from the callback response so the agent
can use it for subsequent REST calls.

## Preconditions

- The store must be running with real GitHub OAuth configured, i.e. `docker compose -f
  compose.yaml up -d` (not `compose.debug.yaml`, which swaps in a GitHub OAuth stub for
  scripted/e2e use only — don't reach for the stub just to avoid a manual browser step).
- `GITHUB_CLIENT_ID`/`GITHUB_CLIENT_SECRET` must be set in the repo-root `.env` (gitignored) for
  the store container to complete the real token exchange.
- The acting GitHub login must already map to a `stakeholders` row via its `github_login` column,
  or the callback will 401 with `No stakeholder mapped to GitHub login: <login>`. There is no API
  to self-register — insert directly into Postgres if needed:
  ```sh
  docker exec spool-postgres-1 psql -U spool -d spool -c \
    "INSERT INTO stakeholders (name, email, role, discipline, github_login) \
     VALUES ('<name>', '<email>', 'stakeholder', '<discipline>', '<github-login>') \
     ON CONFLICT (github_login) DO NOTHING RETURNING id;"
  ```
  `discipline` must be one of `product`, `architecture`, `design`, `engineering`, `security`,
  `governance`. Ask the user for these values rather than guessing; do not put an id/label the
  user didn't provide.

## Step-by-step

1. Confirm the store is reachable on its published port (default `http://localhost:3002`, per
   `compose.yaml`'s `3002:3000` mapping — adjust if the user's compose setup differs).
2. Give the user the login URL to open in a browser:
   ```
   http://localhost:3002/auth/github/login
   ```
   Omit `workspaceId` for a workspace-less bootstrap token — this is required the first time,
   since a fresh stakeholder has no memberships yet and endpoints like `POST /workspaces` need
   exactly that kind of token. Only add `?workspaceId=<id>` once the user already belongs to that
   workspace and needs a workspace-scoped token (e.g. to call
   `POST /workspaces/:id/members`).
3. Ask the user (via `ask_user`, not free-text) to paste the `sessionToken` value from the JSON
   the callback page renders (`{"sessionToken": "...", "refreshToken": "...", "expiresAt": ...}`).
   The refresh token isn't needed for a single one-off call; only prompt for it if you intend to
   refresh the session later without repeating the browser step.
4. If the callback instead shows a 401 `No stakeholder mapped to GitHub login: <login>`, stop and
   follow the stakeholder-insert step under Preconditions, then have the user redo step 2's login
   URL (session tokens embed `stakeholderId`, so the earlier attempt cannot be reused).
5. Use the returned token as `Authorization: Bearer <sessionToken>` on subsequent REST calls
   (e.g. `curl -H "Authorization: Bearer <token>" ...`). Session tokens are short-lived (minutes,
   per `auth-config.ts`'s TTL) — if a call later 401s, repeat this flow rather than assuming the
   token is still valid.

## Checklist

- [ ] Never fall back to the `github-oauth-stub` (compose.debug.yaml) just to avoid asking the
      user to authenticate manually — it fakes GitHub identity and does not represent this user.
- [ ] Never guess or fabricate a `sessionToken` — always obtain it from the user via `ask_user`
      after they've completed the real browser login.
- [ ] Don't reuse a token across a different `workspaceId` scope than it was minted for; mint a
      new one if you need a workspace-bound token (see step 2).
