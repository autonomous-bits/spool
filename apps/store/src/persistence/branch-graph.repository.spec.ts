/**
 * Unit tests for the pure branch-delta resolution functions
 * (`resolveChunkDelta`, `resolveEdgeDelta`) that back `BranchGraphRepository`
 * (story S02). No database involved — these prove the merge/override/delete
 * semantics from the "Edge-delta resolution matrix" directly.
 */

import { describe, expect, it } from 'vitest';
import { resolveChunkDelta, resolveEdgeDelta } from './branch-graph.repository.js';
import type { PersistedChunk } from './chunk-graph.repository.js';
import { chunkLifecycleStatus } from '../domain/chunk-lifecycle.js';
import {
  createEdge,
  deactivateEdge,
  supersedeEdge,
  EdgeLineageError,
} from '../domain/edge-lineage.js';
import { ideaLabel, workspaceId } from '../domain/types/index.js';

const ws = workspaceId('ws-branch-delta-test');
const branchId = 'branch-1' as import('../domain/types/index.js').BranchId;
const label = ideaLabel('IDEA-branch-delta');
const source = ideaLabel('IDEA-source');
const target = ideaLabel('IDEA-target');

function mainlineChunk(content: string): PersistedChunk {
  return {
    workspaceId: ws,
    ideaLabel: label,
    chunkType: 'feature',
    discipline: 'engineering',
    contextKind: 'permanent',
    content,
    status: chunkLifecycleStatus('approved', 'active'),
  };
}

function branchChunk(content: string): PersistedChunk {
  return {
    workspaceId: ws,
    ideaLabel: label,
    chunkType: 'feature',
    discipline: 'engineering',
    contextKind: 'permanent',
    content,
    status: chunkLifecycleStatus('draft', 'active'),
  };
}

describe('resolveChunkDelta', () => {
  it('returns the mainline chunk unchanged when there is no branch delta', () => {
    const mainline = mainlineChunk('approved content');
    expect(resolveChunkDelta(mainline, undefined)).toBe(mainline);
  });

  it('returns undefined when there is neither a mainline chunk nor a delta', () => {
    expect(resolveChunkDelta(undefined, undefined)).toBeUndefined();
  });

  it("returns the branch's own chunk for an 'upsert' delta, overriding mainline", () => {
    const mainline = mainlineChunk('approved content');
    const draft = branchChunk('branch draft content');
    const resolved = resolveChunkDelta(mainline, {
      workspaceId: ws,
      branchId,
      ideaLabel: label,
      deltaKind: 'upsert',
      chunk: draft,
    });
    expect(resolved).toBe(draft);
  });

  it("returns the branch's own chunk for a pure addition ('upsert' with no mainline chunk)", () => {
    const draft = branchChunk('brand new branch-only idea');
    const resolved = resolveChunkDelta(undefined, {
      workspaceId: ws,
      branchId,
      ideaLabel: label,
      deltaKind: 'upsert',
      chunk: draft,
    });
    expect(resolved).toBe(draft);
  });

  it("omits the chunk from the resolved view for a 'delete' delta, regardless of mainline", () => {
    const mainline = mainlineChunk('approved content');
    const resolved = resolveChunkDelta(mainline, {
      workspaceId: ws,
      branchId,
      ideaLabel: label,
      deltaKind: 'delete',
    });
    expect(resolved).toBeUndefined();
  });

  it("throws if an 'upsert' delta is missing its chunk payload", () => {
    expect(() =>
      resolveChunkDelta(undefined, {
        workspaceId: ws,
        branchId,
        ideaLabel: label,
        deltaKind: 'upsert',
      }),
    ).toThrow(/no chunk payload/);
  });
});

describe('resolveEdgeDelta', () => {
  const identity = {
    workspaceId: ws,
    sourceLabel: source,
    targetLabel: target,
    relationshipType: 'depends-on' as const,
  };

  it('returns the mainline lineage unchanged when there is no branch delta', () => {
    const mainline = createEdge(ws, source, target, 'depends-on');
    expect(resolveEdgeDelta(identity, mainline, undefined)).toBe(mainline);
  });

  it('returns undefined when neither mainline nor a delta exists for the identity', () => {
    expect(resolveEdgeDelta(identity, undefined, undefined)).toBeUndefined();
  });

  it("creates a new active edge for a pure addition ('upsert' with no mainline lineage)", () => {
    const resolved = resolveEdgeDelta(identity, undefined, {
      ...identity,
      branchId,
      deltaKind: 'upsert',
    });
    expect(resolved).toBeDefined();
    expect(resolved!.versions).toHaveLength(1);
    expect(resolved!.versions[0]?.state).toBe('active');
  });

  it("returns the mainline lineage unchanged for 'upsert' over an already-active mainline edge (idempotent)", () => {
    const mainline = createEdge(ws, source, target, 'depends-on');
    const resolved = resolveEdgeDelta(identity, mainline, {
      ...identity,
      branchId,
      deltaKind: 'upsert',
    });
    expect(resolved).toBe(mainline);
  });

  it("throws invalid-state-transition for 'upsert' over a deactivated mainline edge", () => {
    const mainline = deactivateEdge(createEdge(ws, source, target, 'depends-on'));
    expect(() =>
      resolveEdgeDelta(identity, mainline, {
        ...identity,
        branchId,
        deltaKind: 'upsert',
      }),
    ).toThrow(EdgeLineageError);
    try {
      resolveEdgeDelta(identity, mainline, {
        ...identity,
        branchId,
        deltaKind: 'upsert',
      });
      expect.fail('expected resolveEdgeDelta to throw');
    } catch (error) {
      expect((error as EdgeLineageError).code).toBe('invalid-state-transition');
    }
  });

  it("deactivates an active mainline edge for a 'deactivate' delta, without mutating the mainline lineage value", () => {
    const mainline = createEdge(ws, source, target, 'depends-on');
    const resolved = resolveEdgeDelta(identity, mainline, {
      ...identity,
      branchId,
      deltaKind: 'deactivate',
    });
    expect(resolved).not.toBe(mainline);
    expect(mainline.versions[0]?.state).toBe('active');
    expect(resolved!.versions[resolved!.versions.length - 1]?.state).toBe('deactivated');
  });

  it("is idempotent for 'deactivate' over an already-deactivated mainline edge", () => {
    const mainline = deactivateEdge(createEdge(ws, source, target, 'depends-on'));
    const resolved = resolveEdgeDelta(identity, mainline, {
      ...identity,
      branchId,
      deltaKind: 'deactivate',
    });
    expect(resolved).toBe(mainline);
  });

  it("creates then deactivates a branch-only edge for 'deactivate' with no mainline lineage", () => {
    const resolved = resolveEdgeDelta(identity, undefined, {
      ...identity,
      branchId,
      deltaKind: 'deactivate',
    });
    expect(resolved).toBeDefined();
    // deactivateEdge appends a new version rather than mutating the
    // freshly-created 'active' version in place: 2 versions total, the
    // first superseded, the last deactivated.
    expect(resolved!.versions).toHaveLength(2);
    expect(resolved!.versions[0]?.state).toBe('superseded');
    expect(resolved!.versions[1]?.state).toBe('deactivated');
  });

  it('preserves a superseded mainline lineage untouched when there is no branch delta', () => {
    let mainline = createEdge(ws, source, target, 'depends-on');
    mainline = supersedeEdge(mainline, identity);
    expect(resolveEdgeDelta(identity, mainline, undefined)).toBe(mainline);
  });
});
