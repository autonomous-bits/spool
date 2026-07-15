import type { FieldDef, Pool, QueryResult, QueryResultRow } from 'pg';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PG_POOL } from './pg-pool.token.js';
import { StakeholderDisciplineRepository } from './stakeholder-discipline.repository.js';

function buildQueryResult<TRow extends QueryResultRow>(rows: TRow[]): QueryResult<TRow> {
  return {
    command: 'SELECT',
    rowCount: rows.length,
    oid: 0,
    rows,
    fields: [] as FieldDef[],
  };
}

describe('StakeholderDisciplineRepository', () => {
  let repository: StakeholderDisciplineRepository;
  let pool: Pick<Pool, 'query'>;

  beforeEach(async () => {
    pool = {
      query: vi.fn<Pool['query']>(),
    } satisfies Pick<Pool, 'query'>;

    const module: TestingModule = await Test.createTestingModule({
      providers: [
        StakeholderDisciplineRepository,
        {
          provide: PG_POOL,
          useValue: pool,
        },
      ],
    }).compile();

    repository = module.get(StakeholderDisciplineRepository);
  });

  it('assign inserts or reuses the row and returns the persisted createdAt', async () => {
    vi.mocked(pool.query)
      .mockResolvedValueOnce(buildQueryResult([]))
      .mockResolvedValueOnce(
        buildQueryResult([
          {
            workspace_id: 'workspace-1',
            stakeholder_id: 'stakeholder-1',
            discipline: 'security',
            created_at: new Date('2026-07-15T07:00:00.000Z'),
          },
        ]),
      );

    await expect(repository.assign('workspace-1', 'stakeholder-1', 'security')).resolves.toEqual({
      workspaceId: 'workspace-1',
      stakeholderId: 'stakeholder-1',
      discipline: 'security',
      createdAt: new Date('2026-07-15T07:00:00.000Z'),
    });

    expect(pool.query).toHaveBeenNthCalledWith(
      1,
      expect.stringContaining('INSERT INTO stakeholder_disciplines'),
      ['workspace-1', 'stakeholder-1', 'security'],
    );
    expect(pool.query).toHaveBeenNthCalledWith(
      2,
      expect.stringContaining('SELECT workspace_id, stakeholder_id, discipline, created_at'),
      ['workspace-1', 'stakeholder-1', 'security'],
    );
  });

  it('assign throws if the row cannot be re-selected after insert/select round-trip', async () => {
    vi.mocked(pool.query)
      .mockResolvedValueOnce(buildQueryResult([]))
      .mockResolvedValueOnce(buildQueryResult([]));

    await expect(repository.assign('workspace-1', 'stakeholder-1', 'security')).rejects.toThrow(
      'expected stakeholder discipline row after insert/select round-trip',
    );
  });

  it('revoke returns true when a row was deleted', async () => {
    vi.mocked(pool.query).mockResolvedValue(buildQueryResult([{ '?column?': 1 }]));

    await expect(repository.revoke('workspace-1', 'stakeholder-1', 'security')).resolves.toBe(true);

    expect(pool.query).toHaveBeenCalledWith(
      expect.stringContaining('DELETE FROM stakeholder_disciplines'),
      ['workspace-1', 'stakeholder-1', 'security'],
    );
  });

  it('revoke returns false when no row matched', async () => {
    vi.mocked(pool.query).mockResolvedValue(buildQueryResult([]));

    await expect(repository.revoke('workspace-1', 'stakeholder-1', 'security')).resolves.toBe(false);
  });
});
