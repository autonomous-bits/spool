#!/usr/bin/env node
/**
 * Dev/test helper: drives the GitHub OAuth login/callback flow (Meridian IDEA-81, G04.SG0)
 * against a running `spoolstore` instance and prints a ready-to-use session token.
 *
 * Intended for local Docker Compose verification (`docker compose up --build spoolstore`,
 * which wires `HttpGithubOAuthClient` to the `github-oauth-stub` service — see compose.yaml)
 * and manual/ad-hoc testing. Not used by any automated test suite and not imported by
 * application code (tools/ is shared scripts only, per docs/constitution.md).
 *
 * Usage:
 *   node tools/dev-session-token/get-dev-session-token.mjs
 *   node tools/dev-session-token/get-dev-session-token.mjs --create-branch my-branch engineering
 *
 * Env vars:
 *   STORE_URL           base URL of the running store (default http://localhost:3000)
 *   OAUTH_CODE           value sent as the callback's `code` query param (default "dev-code";
 *                        the stub's HttpGithubOAuthClient path ignores its value, but Fake
 *                        clients used in tests may require an exact match)
 *   STAKEHOLDER_ID       stakeholderId to use for --create-branch (default: the OAuth e2e
 *                        fixture stakeholder seeded by migration 0006, discipline "engineering")
 */

import { parseArgs } from 'node:util';

const STORE_URL = process.env.STORE_URL ?? 'http://localhost:3000';
const OAUTH_CODE = process.env.OAUTH_CODE ?? 'dev-code';
const STAKEHOLDER_ID =
  process.env.STAKEHOLDER_ID ?? '00000000-0000-0000-0000-000000000002';

/**
 * Decodes (without verifying) the store's HMAC-token envelope — `${base64url(JSON payload)}.
 * ${base64url(signature)}`, per `apps/store/src/auth/hmac-token.ts` — to show the token's claims
 * for convenience. This is NOT a JWT.
 */
/**
 * @param {string} token
 * @returns {unknown}
 */
function decodeTokenEnvelope(token) {
  const [payload] = token.split('.');
  if (payload === undefined) {
    return undefined;
  }
  try {
    return /** @type {unknown} */ (JSON.parse(Buffer.from(payload, 'base64url').toString('utf8')));
  } catch {
    return undefined;
  }
}

async function fetchLoginState() {
  const response = await fetch(`${STORE_URL}/auth/github/login`, { redirect: 'manual' });
  const location = response.headers.get('location');
  if (location === null) {
    throw new Error(
      `GET /auth/github/login did not return a redirect (status ${String(response.status)}); is the store running at ${STORE_URL}?`,
    );
  }
  const state = new URL(location).searchParams.get('state');
  if (state === null) {
    throw new Error(`Login redirect had no state param: ${location}`);
  }
  return state;
}

/**
 * @param {string} state
 * @returns {Promise<string>}
 */
async function fetchSessionToken(state) {
  const url = new URL(`${STORE_URL}/auth/github/callback`);
  url.searchParams.set('code', OAUTH_CODE);
  url.searchParams.set('state', state);

  const response = await fetch(url);
  const body = /** @type {{ sessionToken?: string }} */ (await response.json());
  if (!response.ok) {
    throw new Error(
      `GET /auth/github/callback failed with ${String(response.status)}: ${JSON.stringify(body)}`,
    );
  }
  return /** @type {string} */ (body.sessionToken);
}

/**
 * @param {string} sessionToken
 * @param {string} name
 * @param {string} discipline
 * @returns {Promise<unknown>}
 */
async function createBranch(sessionToken, name, discipline) {
  const response = await fetch(`${STORE_URL}/branches`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, discipline, stakeholderId: STAKEHOLDER_ID }),
  });
  const body = /** @type {unknown} */ (await response.json());
  if (!response.ok) {
    throw new Error(`POST /branches failed with ${String(response.status)}: ${JSON.stringify(body)}`);
  }
  return body;
}

async function main() {
  const { values, positionals } = parseArgs({
    options: {
      'create-branch': { type: 'boolean', default: false },
    },
    allowPositionals: true,
  });

  const state = await fetchLoginState();
  const sessionToken = await fetchSessionToken(state);
  const claims = decodeTokenEnvelope(sessionToken);

  console.log(`sessionToken: ${sessionToken}`);
  if (claims !== undefined) {
    console.log(`claims: ${JSON.stringify(claims)}`);
  }
  console.log(`\nUse it as: Authorization: ${['Bearer', sessionToken].join(' ')}`);

  if (values['create-branch']) {
    const [name = 'dev-session-token-branch', discipline = 'engineering'] = positionals;
    const branch = await createBranch(sessionToken, name, discipline);
    console.log(`\nCreated branch: ${JSON.stringify(branch, null, 2)}`);
  }
}

main().catch((/** @type {unknown} */ error) => {
  console.error(error instanceof Error ? error.message : error);
  process.exitCode = 1;
});
