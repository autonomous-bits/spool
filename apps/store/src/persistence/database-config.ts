/**
 * Fail-fast Postgres connection configuration for the store's persistence
 * adapter.
 *
 * Sources of authority:
 * - Constitution II: "Runtime dependencies such as Postgres MUST be supplied
 *   through the repository's container configuration."
 * - nestjs-security skill: "Read secrets from environment ... Fail fast at
 *   startup when required configuration is absent."
 *
 * There are no hardcoded defaults for credentials: every required value must
 * come from the environment, and a missing value throws immediately rather
 * than silently connecting with an implicit default.
 */

export interface DatabaseConfig {
  readonly host: string;
  readonly port: number;
  readonly user: string;
  readonly password: string;
  readonly database: string;
}

export class DatabaseConfigError extends Error {
  override readonly name = 'DatabaseConfigError';

  constructor(missingVariable: string) {
    super(
      `Missing required environment variable '${missingVariable}' for the store's Postgres connection. See config/store.env.example.`,
    );
  }
}

function requireEnv(env: NodeJS.ProcessEnv, key: string): string {
  const value = env[key];
  if (value === undefined || value.trim() === '') {
    throw new DatabaseConfigError(key);
  }
  return value;
}

function requirePortEnv(env: NodeJS.ProcessEnv, key: string): number {
  const raw = requireEnv(env, key);
  const port = Number(raw);
  if (!Number.isInteger(port) || port <= 0 || port > 65_535) {
    throw new DatabaseConfigError(key);
  }
  return port;
}

/**
 * Loads and validates the store's Postgres connection configuration from
 * environment variables. Throws {@link DatabaseConfigError} if any required
 * variable is absent or malformed.
 */
export function loadDatabaseConfig(
  env: NodeJS.ProcessEnv = process.env,
): DatabaseConfig {
  return Object.freeze({
    host: requireEnv(env, 'STORE_DB_HOST'),
    port: requirePortEnv(env, 'STORE_DB_PORT'),
    user: requireEnv(env, 'STORE_DB_USER'),
    password: requireEnv(env, 'STORE_DB_PASSWORD'),
    database: requireEnv(env, 'STORE_DB_NAME'),
  });
}
