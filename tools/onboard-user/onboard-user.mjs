#!/usr/bin/env node
/**
 * Interactive dev/ops helper to onboard a new user into Spool by driving the store's human-only
 * workspace endpoints (Meridian IDEA-94/IDEA-88/IDEA-95): `POST /workspaces` and
 * `POST /workspaces/:id/members`. Deliberately a plain script, not an MCP tool — those two
 * endpoints are intentionally human-only and have no MCP equivalent (see
 * apps/store/src/workspaces/workspaces.controller.ts).
 *
 * Guides the caller through one of two flows:
 *   1. Create a brand-new workspace (the caller becomes its first member).
 *   2. Add an existing stakeholder as a member of an existing workspace.
 *
 * Both flows require a valid session token for the *acting* stakeholder. There is no headless
 * way to mint one against the real GitHub OAuth App wired in `compose.yaml` — a store-issued
 * session token only exists after a real browser GitHub login (see
 * apps/store/src/auth/auth.controller.ts). If `SESSION_TOKEN` isn't set, this script prints the
 * `GET /auth/github/login` URL for the caller to open in a browser and prompts them to paste
 * back the `sessionToken` from the callback's JSON response — it never fabricates or reuses a
 * token itself. (For scripted/e2e use only, `tools/dev-session-token` drives the same flow
 * against the `github-oauth-stub` swapped in by `compose.debug.yaml`.)
 *
 * Not used by any automated test suite and not imported by application code (tools/ is shared
 * scripts only, per docs/constitution.md).
 *
 * Usage (interactive):
 *   node tools/onboard-user/onboard-user.mjs
 *
 * Usage (non-interactive, scriptable):
 *   node tools/onboard-user/onboard-user.mjs --create-workspace --name "My Workspace"
 *   node tools/onboard-user/onboard-user.mjs --add-member --workspace-id <id> --stakeholder-id <id>
 *
 * Env vars:
 *   STORE_URL       base URL of the running store (default http://localhost:3000; use
 *                    http://localhost:3002 for the default `compose.yaml` port mapping)
 *   SESSION_TOKEN    session token for the acting stakeholder (optional — if unset, you'll be
 *                    walked through the real GitHub OAuth login to obtain one)
 */

import { createInterface } from 'node:readline/promises';
import { parseArgs } from 'node:util';

const STORE_URL = process.env.STORE_URL ?? 'http://localhost:3000';

/**
 * Reads one line of answer for `message`, via a shared async-iterator over the readline
 * interface rather than repeated `rl.question()` calls. `rl.question()` re-attaches a one-shot
 * 'line' listener on each call, which drops any input line that arrives on non-TTY (piped) stdin
 * between two calls before the next listener is attached — the iterator has no such gap.
 *
 * @param {AsyncIterator<string>} lines
 * @param {string} message
 * @returns {Promise<string>}
 */
async function prompt(lines, message) {
  process.stdout.write(message);
  /** @type {IteratorResult<string>} */
  const result = await lines.next();
  if (result.done === true) {
    throw new Error('No more input available (stdin closed).');
  }
  return result.value.trim();
}

/**
 * Builds the real GitHub OAuth login URL (`GET /auth/github/login`), per
 * `apps/store/src/auth/auth.controller.ts`. Omit `workspaceId` for a workspace-less bootstrap
 * token (needed the first time, since a fresh stakeholder has no memberships yet); pass it to
 * mint a token scoped to a workspace the caller already belongs to (required for
 * `POST /workspaces/:id/members`, which checks the token's `workspaceId` claim against the
 * `X-Workspace-Id` header).
 *
 * @param {string | undefined} workspaceId
 * @returns {string}
 */
function buildLoginUrl(workspaceId) {
  const url = new URL('/auth/github/login', STORE_URL);
  if (workspaceId !== undefined) {
    url.searchParams.set('workspaceId', workspaceId);
  }
  return url.toString();
}

/**
 * Walks the caller through the real (non-headless) GitHub OAuth login: prints the login URL for
 * them to open in a browser, then asks them to paste back the session token from the
 * callback's JSON response. Never fabricates, reuses, or falls back to a stub token \u2014 per
 * spool-local-dev-auth, only a real browser login produces a valid session token.
 *
 * @param {AsyncIterator<string>} lines
 * @param {string | undefined} workspaceId
 * @returns {Promise<string>}
 */
async function promptForSessionTokenViaLogin(lines, workspaceId) {
  const loginUrl = buildLoginUrl(workspaceId);
  console.log(
    `\nNo SESSION_TOKEN set \u2014 open this URL in your browser and complete GitHub login:\n  ${loginUrl}\n` +
      'Then paste the callback response\'s "sessionToken" value below.\n' +
      '(If the callback shows 401 "No stakeholder mapped to GitHub login: <login>", an admin ' +
      'must add that GitHub login to the stakeholders table before you can proceed.)',
  );
  const token = await prompt(lines, 'Session token: ');
  if (token.length === 0) {
    throw new Error('A session token is required to proceed.');
  }
  return token;
}

/**
 * @param {string} sessionToken
 * @returns {Record<string, string>}
 */
function authHeaders(sessionToken) {
  return { Authorization: `Bearer ${sessionToken}` };
}

/**
 * @param {string} name
 * @param {string} sessionToken
 * @returns {Promise<unknown>}
 */
async function createWorkspace(name, sessionToken) {
  const response = await fetch(`${STORE_URL}/workspaces`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...authHeaders(sessionToken),
    },
    body: JSON.stringify({ name }),
  });
  const body = /** @type {unknown} */ (await response.json());
  if (!response.ok) {
    throw new Error(`POST /workspaces failed with ${String(response.status)}: ${JSON.stringify(body)}`);
  }
  return body;
}

/**
 * @param {string} workspaceId
 * @param {string} stakeholderId
 * @param {string} sessionToken
 * @returns {Promise<unknown>}
 */
async function addMember(workspaceId, stakeholderId, sessionToken) {
  const response = await fetch(`${STORE_URL}/workspaces/${encodeURIComponent(workspaceId)}/members`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Workspace-Id': workspaceId,
      ...authHeaders(sessionToken),
    },
    body: JSON.stringify({ stakeholderId }),
  });
  const body = /** @type {unknown} */ (await response.json());
  if (!response.ok) {
    throw new Error(
      `POST /workspaces/${workspaceId}/members failed with ${String(response.status)}: ${JSON.stringify(body)}`,
    );
  }
  return body;
}

/**
 * @param {AsyncIterator<string>} lines
 * @returns {Promise<'create-workspace' | 'add-member'>}
 */
async function promptForFlow(lines) {
  const answer = await prompt(
    lines,
    '\nWhat would you like to do?\n' +
      '  [1] Create a new workspace\n' +
      '  [2] Add a user to an existing workspace\n' +
      'Enter 1 or 2: ',
  );
  if (answer === '1') {
    return 'create-workspace';
  }
  if (answer === '2') {
    return 'add-member';
  }
  throw new Error(`Unrecognized choice: ${answer}`);
}

/**
 * @param {AsyncIterator<string>} lines
 * @returns {Promise<void>}
 */
async function runInteractive(lines) {
  const flow = await promptForFlow(lines);
  const envToken = process.env.SESSION_TOKEN;

  if (flow === 'create-workspace') {
    // Workspace-less bootstrap token: creating a workspace necessarily precedes membership in it.
    const sessionToken = envToken ?? (await promptForSessionTokenViaLogin(lines, undefined));
    const name = await prompt(lines, 'Workspace name: ');
    const workspace = await createWorkspace(name, sessionToken);
    console.log(`\nCreated workspace:\n${JSON.stringify(workspace, null, 2)}`);
    return;
  }

  const workspaceId = await prompt(lines, 'Workspace id: ');
  // Adding a member requires a token scoped to that workspace (X-Workspace-Id must match the
  // token's workspaceId claim), so the login URL below is minted with this workspaceId.
  const sessionToken = envToken ?? (await promptForSessionTokenViaLogin(lines, workspaceId));
  const stakeholderId = await prompt(lines, 'Stakeholder id to add: ');
  const membership = await addMember(workspaceId, stakeholderId, sessionToken);
  console.log(`\nAdded member:\n${JSON.stringify(membership, null, 2)}`);
}

async function main() {
  const { values } = parseArgs({
    options: {
      'create-workspace': { type: 'boolean', default: false },
      'add-member': { type: 'boolean', default: false },
      name: { type: 'string' },
      'workspace-id': { type: 'string' },
      'stakeholder-id': { type: 'string' },
    },
  });

  const rl = createInterface({ input: process.stdin, output: process.stdout });
  const lines = rl[Symbol.asyncIterator]();
  try {
    const envToken = process.env.SESSION_TOKEN;

    if (values['create-workspace']) {
      if (values.name === undefined || values.name.trim().length === 0) {
        throw new Error('--create-workspace requires --name "<workspace name>"');
      }
      const sessionToken = envToken ?? (await promptForSessionTokenViaLogin(lines, undefined));
      const workspace = await createWorkspace(values.name, sessionToken);
      console.log(JSON.stringify(workspace, null, 2));
      return;
    }

    if (values['add-member']) {
      if (values['workspace-id'] === undefined || values['stakeholder-id'] === undefined) {
        throw new Error('--add-member requires --workspace-id <id> and --stakeholder-id <id>');
      }
      const sessionToken =
        envToken ?? (await promptForSessionTokenViaLogin(lines, values['workspace-id']));
      const membership = await addMember(values['workspace-id'], values['stakeholder-id'], sessionToken);
      console.log(JSON.stringify(membership, null, 2));
      return;
    }

    await runInteractive(lines);
  } finally {
    rl.close();
  }
}

main().catch((/** @type {unknown} */ error) => {
  console.error(error instanceof Error ? error.message : error);
  process.exitCode = 1;
});
