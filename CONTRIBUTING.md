# Contributing to Spool

Thanks for your interest in contributing! This guide covers the basics for getting set up and submitting changes.

## Prerequisites

- Node.js >= 24
- [pnpm](https://pnpm.io) 11 (`pnpm@11.0.0`, matching `packageManager` in `package.json`)
- Docker + Docker Compose (for running `apps/store` locally)

## Getting started

```sh
git clone <repo-url>
cd spool
pnpm install
```

This is a pnpm workspace monorepo. Key apps:

- `apps/store` — NestJS knowledge store (chunks, graph edges, branching, lifecycle)
- `apps/mcp` — MCP server for agent-facing harness interactions

See `apps/store/AGENTS.md` and `apps/mcp/AGENTS.md` for app-specific details, and `docs/architecture` for system design.

## Running the store locally

Run `apps/store` via Docker Compose rather than directly on the host:

```sh
docker compose up --build spoolstore
```

Rebuild locally first (`pnpm --filter store build` or `pnpm build`) before `docker compose up --build`, since the Dockerfile copies the pre-built `dist/` rather than compiling in-container.

### Setting up the GitHub OAuth App

The store authenticates stakeholders via a GitHub OAuth App, so you need real OAuth App
credentials before `spoolstore` will start, even for local development.

1. In GitHub, go to **Settings > Developer settings > OAuth Apps > New OAuth App** (or reuse an
   existing dev-only app).
2. Fill in the app details. The exact **Homepage URL** doesn't matter for local dev; set the
   **Authorization callback URL** to:
   ```
   http://localhost:3002/auth/github/callback
   ```
3. Register the app and generate a new **Client Secret**.
4. Create an untracked `.env` file at the repo root (never commit it) with the credentials:
   ```sh
   GITHUB_CLIENT_ID=<your-oauth-app-client-id>
   GITHUB_CLIENT_SECRET=<your-oauth-app-client-secret>
   ```
   `compose.yaml` reads these two variables for the containerized `spoolstore` service.
5. Start the store as usual:
   ```sh
   pnpm --filter store build
   docker compose up --build spoolstore
   ```

With plain `docker compose up --build spoolstore` (using `compose.yaml`), the OAuth flow runs
end-to-end against real `github.com`/`api.github.com` — you'll need to click through a real
GitHub consent screen in your browser at `http://localhost:3002/auth/github/login` (or whichever
route initiates `/authorize`), and the resulting session is tied to your actual GitHub identity.

To mint a session token for local testing without manually driving the OAuth redirect/callback
flow yourself, use the dev helper:

```sh
pnpm dev:session-token
```

See `tools/dev-session-token/README.md` for available options (including `--create-branch`).

### Local stub services under `tools/docker`

`tools/docker` contains two stand-ins for services `spoolstore` would otherwise depend on live,
external HTTPS endpoints for:

- `github-oauth-stub` (`tools/docker/github-oauth-stub`) — stands in for github.com's OAuth
  token-exchange endpoint and api.github.com's `/user` endpoint, since a live interactive GitHub
  consent screen can't be automated in tests. It always resolves to a fixture stakeholder
  (`spool-e2e-oauth-fixture`), skipping the real consent screen.
- `webhook-receiver-stub` (`tools/docker/webhook-receiver-stub`) — a TLS-terminating stand-in for
  a real downstream consumer's HTTPS webhook endpoint. `DeliverySubscription.url` requires
  `https://`, so `DeliveryWorkerService`'s outbound fetch needs a genuine TLS handshake to
  exercise, which is why this stub (unlike `github-oauth-stub`) terminates real TLS rather than
  plain HTTP.

**When to use which compose file:**

- `compose.yaml` (default, `docker compose up --build spoolstore`) — runs the full OAuth
  authorization-code flow against real GitHub (no `github-oauth-stub`), plus
  `webhook-receiver-stub` for webhook delivery. Use this for regular local development and for
  manually testing a real end-to-end GitHub login with your own OAuth App.
- `compose.debug.yaml` (`docker compose -f compose.debug.yaml up --build spoolstore`) — runs
  `github-oauth-stub` instead of real GitHub (`GITHUB_OAUTH_TOKEN_URL`/`GITHUB_USER_API_URL`
  point at it) so you can exercise auth-dependent flows without a live consent screen, along with
  the Node inspector on port 9229. It omits `webhook-receiver-stub`. Use this when debugging or
  when you want to skip the GitHub consent screen entirely; real `GITHUB_CLIENT_ID`/
  `GITHUB_CLIENT_SECRET` are not required in this mode since the token-exchange/user-lookup calls
  never reach real GitHub.

#### Generating the webhook-receiver-stub's TLS cert/key

`webhook-receiver-stub` needs a `cert.pem`/`key.pem` pair to terminate TLS. These are
self-signed, gitignored (`*.pem`), and **not committed** — generate your own locally:

```sh
cd tools/docker/webhook-receiver-stub
openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem -days 825 -nodes \
  -subj "/CN=webhook-receiver-stub" \
  -addext "subjectAltName=DNS:webhook-receiver-stub,DNS:localhost"
chmod 600 key.pem
```

`spoolstore` trusts this same cert via `NODE_EXTRA_CA_CERTS` (it's baked into the `spoolstore`
image too, via the `certs` additional build context in `compose.yaml`), so
`DeliveryWorkerService` can complete a genuine TLS handshake against the stub.

Never commit `key.pem` or `cert.pem` — they're already covered by `.gitignore`, but double-check
before committing docker/webhook-receiver-stub changes. If a private key is ever accidentally
committed or pushed, treat it as compromised and regenerate a fresh pair with the command above,
even if the exposing commit is later rewritten out of history.

## Making changes

1. Create a branch off `main` with a descriptive name.
2. Make focused, well-scoped changes. Avoid unrelated refactors in the same PR.
3. Add or update tests for any behavior you change.
4. Update documentation (README, AGENTS.md, docs/) when it's directly affected by your change.

## Validating your changes

Run the checks relevant to what you touched, and the full workspace checks before opening a PR:

```sh
# Whole workspace
pnpm build
pnpm typecheck
pnpm test

# Store app only
pnpm --filter store typecheck
pnpm --filter store build
pnpm test:store

# MCP app only
pnpm --filter mcp typecheck
pnpm --filter mcp build
pnpm test:mcp
```

Lint and format:

```sh
pnpm lint
pnpm format:check
```

## Commit and PR guidelines

- Write clear, descriptive commit messages explaining *why*, not just *what*.
- Keep PRs small and reviewable; split unrelated changes into separate PRs.
- Ensure `pnpm build`, `pnpm typecheck`, and `pnpm test` all pass before requesting review.
- Link related issues or design docs in the PR description.
- Be responsive to review feedback — the codebase favors precise, surgical changes over broad rewrites.

## Code of conduct

Be respectful and constructive in issues, PRs, and reviews. Assume good intent, and focus feedback on the code, not the author.

## Questions

If anything here is unclear or out of date, open an issue or ask in your PR — this document should evolve with the project.
