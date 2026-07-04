import { describe, expect, it } from 'vitest';
import { DatabaseConfigError, loadDatabaseConfig } from './database-config.js';

function baseEnv(): NodeJS.ProcessEnv {
  return {
    STORE_DB_HOST: 'localhost',
    STORE_DB_PORT: '5433',
    STORE_DB_USER: 'spool',
    STORE_DB_PASSWORD: 'spool_dev',
    STORE_DB_NAME: 'spool',
  };
}

describe('loadDatabaseConfig', () => {
  it('loads a complete configuration from environment variables', () => {
    const config = loadDatabaseConfig(baseEnv());

    expect(config).toEqual({
      host: 'localhost',
      port: 5433,
      user: 'spool',
      password: 'spool_dev',
      database: 'spool',
    });
  });

  it.each([
    'STORE_DB_HOST',
    'STORE_DB_PORT',
    'STORE_DB_USER',
    'STORE_DB_PASSWORD',
    'STORE_DB_NAME',
  ])('throws DatabaseConfigError when %s is missing', (missingKey) => {
    const env = baseEnv();
    delete env[missingKey];

    expect(() => loadDatabaseConfig(env)).toThrow(DatabaseConfigError);
  });

  it('throws when STORE_DB_HOST is blank', () => {
    const env = { ...baseEnv(), STORE_DB_HOST: '   ' };

    expect(() => loadDatabaseConfig(env)).toThrow(DatabaseConfigError);
  });

  it('throws when STORE_DB_PORT is not a valid port number', () => {
    const env = { ...baseEnv(), STORE_DB_PORT: 'not-a-port' };

    expect(() => loadDatabaseConfig(env)).toThrow(DatabaseConfigError);
  });

  it('throws when STORE_DB_PORT is out of range', () => {
    const env = { ...baseEnv(), STORE_DB_PORT: '70000' };

    expect(() => loadDatabaseConfig(env)).toThrow(DatabaseConfigError);
  });
});
