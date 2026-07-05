import { createServer } from 'node:http';

/**
 * Minimal stand-in for github.com's OAuth token-exchange endpoint and api.github.com's /user
 * endpoint, used only by Docker Compose for G04.SG5's containerized end-to-end exercise. A live,
 * interactive GitHub consent screen cannot be automated in that exercise (see goal.html's scope
 * note), so `HttpGithubOAuthClient`'s base URLs are pointed at this stub instead via
 * GITHUB_OAUTH_TOKEN_URL / GITHUB_USER_API_URL, without swapping the client implementation.
 *
 * Deliberately dependency-free (plain node:http) since this is Docker-only test infrastructure,
 * not application code.
 */

const GITHUB_LOGIN = process.env.OAUTH_STUB_GITHUB_LOGIN ?? 'spool-e2e-oauth-fixture';
const ACCESS_TOKEN = 'stub-access-token';
const PORT = Number.parseInt(process.env.PORT ?? '4001', 10);

function readJsonBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on('data', (chunk) => chunks.push(chunk));
    req.on('end', () => {
      if (chunks.length === 0) {
        resolve(undefined);
        return;
      }
      try {
        resolve(JSON.parse(Buffer.concat(chunks).toString('utf8')));
      } catch {
        resolve(undefined);
      }
    });
    req.on('error', reject);
  });
}

function sendJson(res, statusCode, body) {
  const payload = JSON.stringify(body);
  res.writeHead(statusCode, {
    'Content-Type': 'application/json',
    'Content-Length': Buffer.byteLength(payload),
  });
  res.end(payload);
}

const server = createServer(async (req, res) => {
  const url = new URL(req.url ?? '/', 'http://github-oauth-stub.local');

  if (req.method === 'POST' && url.pathname === '/login/oauth/access_token') {
    await readJsonBody(req);
    sendJson(res, 200, { access_token: ACCESS_TOKEN, token_type: 'bearer', scope: 'read:user' });
    return;
  }

  if (req.method === 'GET' && url.pathname === '/user') {
    const authorization = req.headers['authorization'];
    if (authorization !== `Bearer ${ACCESS_TOKEN}`) {
      sendJson(res, 401, { message: 'Bad credentials' });
      return;
    }
    sendJson(res, 200, { login: GITHUB_LOGIN });
    return;
  }

  sendJson(res, 404, { message: 'Not found' });
});

server.listen(PORT, () => {
  // eslint-disable-next-line no-console -- Docker-only stub; container logs are the only observability here.
  console.log(`github-oauth-stub listening on port ${PORT}`);
});
