import { createServer } from 'node:https';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

/**
 * TLS-terminating webhook receiver stub for G14.SG4's Docker end-to-end exercise. Stands in for
 * a real downstream consumer's HTTPS endpoint: `DeliverySubscription.url` must be `https://`
 * (see `apps/store/src/domain/delivery-subscription.ts`), so `DeliveryWorkerService`'s outbound
 * `fetch` needs a genuine TLS handshake to exercise, not a plain-HTTP stand-in.
 *
 * Terminates TLS with a static, repo-committed self-signed cert/key pair (SAN
 * `webhook-receiver-stub`, matching this service's Compose DNS name) so `spoolstore` can trust it
 * via `NODE_EXTRA_CA_CERTS` pointed at the same cert baked into its own image (see compose.yaml).
 *
 * Records every received delivery in memory and exposes it over `GET /received` so the manual
 * Docker exercise can assert on the signed payload and headers `DeliveryWorkerService` sent,
 * without needing to grep container logs.
 *
 * Deliberately dependency-free (plain node:https) since this is Docker-only test infrastructure,
 * not application code.
 */

/** @typedef {import('node:http').IncomingMessage} IncomingMessage */
/** @typedef {import('node:http').ServerResponse} ServerResponse */

const __dirname = dirname(fileURLToPath(import.meta.url));
const PORT = Number.parseInt(process.env.PORT ?? '4443', 10);

/** @type {{ receivedAt: string, headers: Record<string, string | string[] | undefined>, body: unknown }[]} */
const received = [];

/**
 * @param {IncomingMessage} req
 * @returns {Promise<Buffer>}
 */
function readRawBody(req) {
  return new Promise((resolve, reject) => {
    /** @type {Buffer[]} */
    const chunks = [];
    req.on('data', (/** @type {Buffer} */ chunk) => chunks.push(chunk));
    req.on('end', () => { resolve(Buffer.concat(chunks)); });
    req.on('error', reject);
  });
}

/**
 * @param {ServerResponse} res
 * @param {number} statusCode
 * @param {unknown} body
 */
function sendJson(res, statusCode, body) {
  const payload = JSON.stringify(body);
  res.writeHead(statusCode, {
    'Content-Type': 'application/json',
    'Content-Length': Buffer.byteLength(payload),
  });
  res.end(payload);
}

const server = createServer(
  {
    cert: readFileSync(join(__dirname, 'cert.pem')),
    key: readFileSync(join(__dirname, 'key.pem')),
  },
  (req, res) => {
    const url = new URL(req.url ?? '/', 'https://webhook-receiver-stub.local');

    if (req.method === 'POST' && url.pathname === '/webhook') {
      void readRawBody(req).then((raw) => {
        /** @type {unknown} */
        let parsedBody;
        try {
          parsedBody = /** @type {unknown} */ (JSON.parse(raw.toString('utf8')));
        } catch {
          parsedBody = null;
        }
        received.push({
          receivedAt: new Date().toISOString(),
          headers: req.headers,
          body: parsedBody,
        });
        console.log(`webhook-receiver-stub received delivery: ${raw.toString('utf8')}`);
        sendJson(res, 200, { ok: true });
      });
      return;
    }

    if (req.method === 'GET' && url.pathname === '/received') {
      sendJson(res, 200, received);
      return;
    }

    sendJson(res, 404, { message: 'Not found' });
  },
);

server.listen(PORT, () => {
  console.log(`webhook-receiver-stub listening on port ${String(PORT)}`);
});
