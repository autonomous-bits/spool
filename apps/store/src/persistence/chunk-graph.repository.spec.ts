import { describe, expect, it } from 'vitest';
import { DatabaseError } from 'pg';
import { mapPersistenceError } from './chunk-graph.repository.js';
import { EdgeLineageError } from '../domain/edge-lineage.js';

function uniqueViolation(constraint: string): DatabaseError {
  const error = new DatabaseError(
    'duplicate key value violates unique constraint',
    0,
    'error',
  );
  error.code = '23505';
  error.constraint = constraint;
  return error;
}

describe('mapPersistenceError', () => {
  it('maps a violation of the "one active edge" partial unique index to duplicate-active-relationship', () => {
    const mapped = mapPersistenceError(
      uniqueViolation('edge_versions_one_active_idx'),
    );

    expect(mapped).toBeInstanceOf(EdgeLineageError);
    expect((mapped as EdgeLineageError).code).toBe(
      'duplicate-active-relationship',
    );
  });

  it('maps any other unique-constraint violation (e.g. the primary key) to lineage-violation', () => {
    const mapped = mapPersistenceError(uniqueViolation('edge_versions_pkey'));

    expect(mapped).toBeInstanceOf(EdgeLineageError);
    expect((mapped as EdgeLineageError).code).toBe('lineage-violation');
  });

  it('passes through errors that are not unique-constraint violations unchanged', () => {
    const domainError = new EdgeLineageError(
      'invalid-state-transition',
      'already an EdgeLineageError',
    );

    expect(mapPersistenceError(domainError)).toBe(domainError);
    expect(mapPersistenceError(new Error('plain error'))).toBeInstanceOf(
      Error,
    );
  });

  it('does not remap a DatabaseError with a different SQLSTATE', () => {
    const notFound = new DatabaseError('relation does not exist', 0, 'error');
    notFound.code = '42P01';

    expect(mapPersistenceError(notFound)).toBe(notFound);
  });
});
