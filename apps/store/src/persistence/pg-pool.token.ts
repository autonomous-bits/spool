/**
 * DI token for the `pg.Pool` instance provided by PersistenceModule. Symbols avoid collisions with
 * other injection tokens and keep the Pool provider decoupled from any single class type.
 */
export const PG_POOL = Symbol('PG_POOL');
