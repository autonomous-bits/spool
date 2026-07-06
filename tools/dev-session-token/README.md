# dev-session-token

Dev/test helper that drives the GitHub OAuth login/callback flow (Meridian IDEA-81, G04.SG0)
against a running `spoolstore` instance and prints a ready-to-use session token, so you don't
have to manually copy `state` out of a redirect and curl the callback by hand.

Requires a running store whose `GithubOAuthClient` resolves callbacks without a live GitHub
consent screen — either:

- `docker compose up --build spoolstore` (wires the real `HttpGithubOAuthClient` to the
  `github-oauth-stub` service, which maps any code to GitHub login `spool-e2e-oauth-fixture`,
  seeded by migration `0006_seed_oauth_e2e_fixture_stakeholder.sql` with a non-null discipline), or
- any other setup where `GITHUB_OAUTH_TOKEN_URL`/`GITHUB_USER_API_URL` point at a stub.

## Usage

From the repo root, with the store running (default `http://localhost:3000`):

```sh
pnpm dev:session-token
```

Prints the minted `sessionToken`, its decoded claims, and a ready-to-paste
`Authorization: Bearer <token>` header.

To also create a draft branch as the fixture stakeholder in the same call:

```sh
pnpm dev:session-token -- --create-branch my-branch engineering
```

## Env vars

| Var | Default | Purpose |
|---|---|---|
| `STORE_URL` | `http://localhost:3000` | Base URL of the running store |
| `OAUTH_CODE` | `dev-code` | Value sent as the callback's `code` param |
| `STAKEHOLDER_ID` | `00000000-0000-0000-0000-000000000002` | stakeholderId used by `--create-branch` |

Not used by any automated test suite; `tools/` is shared scripts only and application code must
not import from it (see `docs/constitution.md`).
