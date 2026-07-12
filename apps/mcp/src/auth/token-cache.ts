import { AsyncEntry } from '@napi-rs/keyring';

export interface CachedCredentials {
  sessionToken: string;
  sessionTokenExpiresAt: number;
  refreshToken: string;
}

export interface TokenCache {
  load(storeUrl: string, workspaceId: string): Promise<CachedCredentials | undefined>;
  save(storeUrl: string, workspaceId: string, credentials: CachedCredentials): Promise<void>;
  clear(storeUrl: string, workspaceId: string): Promise<void>;
}

export interface TokenCacheLogger {
  warn(message: string): void;
}

export interface TokenCacheKeyringClient {
  getPassword(service: string, account: string): Promise<string | null | undefined>;
  setPassword(service: string, account: string, password: string): Promise<void>;
  deletePassword(service: string, account: string): Promise<boolean>;
}

export interface CreateTokenCacheOptions {
  keyring?: TokenCacheKeyringClient;
  logger?: TokenCacheLogger;
  serviceName?: string;
}

export class TokenCacheError extends Error {
  constructor(message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = 'TokenCacheError';
  }
}

const DEFAULT_SERVICE_NAME = 'spool-mcp';

const osKeyringClient: TokenCacheKeyringClient = {
  async getPassword(service: string, account: string): Promise<string | undefined> {
    return new AsyncEntry(service, account).getPassword();
  },
  async setPassword(service: string, account: string, password: string): Promise<void> {
    await new AsyncEntry(service, account).setPassword(password);
  },
  async deletePassword(service: string, account: string): Promise<boolean> {
    return new AsyncEntry(service, account).deletePassword().then((deleted) => Boolean(deleted));
  },
};

function buildAccountName(storeUrl: string, workspaceId: string): string {
  return `${encodeURIComponent(storeUrl)}::${encodeURIComponent(workspaceId)}`;
}

function toLogContext(storeUrl: string, workspaceId: string): string {
  return `storeUrl=${JSON.stringify(storeUrl)} workspaceId=${JSON.stringify(workspaceId)}`;
}

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function parseCachedCredentials(serialized: string): CachedCredentials {
  const parsed: unknown = JSON.parse(serialized);
  if (!isObjectRecord(parsed)) {
    throw new TypeError('cached credentials must be a JSON object');
  }

  const sessionToken = parsed.sessionToken;
  const sessionTokenExpiresAt = parsed.sessionTokenExpiresAt;
  const refreshToken = parsed.refreshToken;

  if (typeof sessionToken !== 'string' || sessionToken.length === 0) {
    throw new TypeError('cached credentials.sessionToken must be a non-empty string');
  }
  if (
    typeof sessionTokenExpiresAt !== 'number' ||
    !Number.isFinite(sessionTokenExpiresAt) ||
    !Number.isInteger(sessionTokenExpiresAt) ||
    sessionTokenExpiresAt <= 0
  ) {
    throw new TypeError('cached credentials.sessionTokenExpiresAt must be a positive integer epoch-seconds value');
  }
  if (typeof refreshToken !== 'string' || refreshToken.length === 0) {
    throw new TypeError('cached credentials.refreshToken must be a non-empty string');
  }

  return { sessionToken, sessionTokenExpiresAt, refreshToken };
}

function wrapKeyringError(action: 'load' | 'save' | 'clear', storeUrl: string, workspaceId: string, error: unknown): TokenCacheError {
  return new TokenCacheError(
    `Failed to ${action} cached Spool MCP credentials in the OS keychain (${toLogContext(storeUrl, workspaceId)})`,
    { cause: error },
  );
}

export function createTokenCache(options: CreateTokenCacheOptions = {}): TokenCache {
  const keyring = options.keyring ?? osKeyringClient;
  const logger = options.logger ?? console;
  const serviceName = options.serviceName ?? DEFAULT_SERVICE_NAME;

  return {
    async load(storeUrl: string, workspaceId: string): Promise<CachedCredentials | undefined> {
      let serialized: string | null | undefined;
      try {
        serialized = await keyring.getPassword(serviceName, buildAccountName(storeUrl, workspaceId));
      } catch (error) {
        throw wrapKeyringError('load', storeUrl, workspaceId, error);
      }

      if (serialized == null) {
        return undefined;
      }

      try {
        return parseCachedCredentials(serialized);
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        logger.warn(
          `Ignoring malformed cached Spool MCP credentials from the OS keychain (${toLogContext(storeUrl, workspaceId)}): ${message}`,
        );
        return undefined;
      }
    },

    async save(storeUrl: string, workspaceId: string, credentials: CachedCredentials): Promise<void> {
      try {
        await keyring.setPassword(
          serviceName,
          buildAccountName(storeUrl, workspaceId),
          JSON.stringify({
            sessionToken: credentials.sessionToken,
            sessionTokenExpiresAt: credentials.sessionTokenExpiresAt,
            refreshToken: credentials.refreshToken,
          } satisfies CachedCredentials),
        );
      } catch (error) {
        throw wrapKeyringError('save', storeUrl, workspaceId, error);
      }
    },

    async clear(storeUrl: string, workspaceId: string): Promise<void> {
      try {
        await keyring.deletePassword(serviceName, buildAccountName(storeUrl, workspaceId));
      } catch (error) {
        throw wrapKeyringError('clear', storeUrl, workspaceId, error);
      }
    },
  };
}

export function createInMemoryTokenCache(): TokenCache {
  const entries = new Map<string, string>();

  return {
    load(storeUrl: string, workspaceId: string): Promise<CachedCredentials | undefined> {
      const cacheKey = `${DEFAULT_SERVICE_NAME}::${buildAccountName(storeUrl, workspaceId)}`;
      const serialized = entries.get(cacheKey);
      if (serialized === undefined) {
        return Promise.resolve(undefined);
      }
      return Promise.resolve(parseCachedCredentials(serialized));
    },

    save(storeUrl: string, workspaceId: string, credentials: CachedCredentials): Promise<void> {
      const cacheKey = `${DEFAULT_SERVICE_NAME}::${buildAccountName(storeUrl, workspaceId)}`;
      entries.set(
        cacheKey,
        JSON.stringify({
          sessionToken: credentials.sessionToken,
          sessionTokenExpiresAt: credentials.sessionTokenExpiresAt,
          refreshToken: credentials.refreshToken,
        } satisfies CachedCredentials),
      );
      return Promise.resolve();
    },

    clear(storeUrl: string, workspaceId: string): Promise<void> {
      const cacheKey = `${DEFAULT_SERVICE_NAME}::${buildAccountName(storeUrl, workspaceId)}`;
      entries.delete(cacheKey);
      return Promise.resolve();
    },
  };
}
