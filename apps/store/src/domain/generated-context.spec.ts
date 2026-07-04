import { describe, expect, it } from 'vitest';
import { ideaLabel, workspaceId, generatedContextId } from './types/index.js';
import { chunkLifecycleStatus } from './chunk-lifecycle.js';
import { createEdge, deactivateEdge, supersedeEdge } from './edge-lineage.js';
import {
  GeneratedContextError,
  includeChunkInGeneratedContext,
  includeRelationshipInGeneratedContext,
  createGeneratedContextPackage,
  type GeneratedContextItem,
  type RelationshipEndpointChunk,
} from './generated-context.js';

/**
 * Tests for generated-context provenance rules and packaging.
 *
 * Story: S08 — Give agents traceable approved implementation context.
 * Sources of authority:
 *   - docs/specifications/feature-01-core-domain-model/stories/S08-traceable-generated-context.md
 *   - docs/specifications/feature-01-core-domain-model/technical-specification.md
 *     §"Generated context", §"Workspace scoping", §"Edge determinism"
 *   - Meridian IDEA-36, IDEA-37, IDEA-38
 */

const ws1 = workspaceId('workspace-1');
const ws2 = workspaceId('workspace-2');
const ideaA = ideaLabel('IDEA-A');
const ideaB = ideaLabel('IDEA-B');
const ideaC = ideaLabel('IDEA-C');

const approvedActive = chunkLifecycleStatus('approved', 'active');
const promotedActive = chunkLifecycleStatus('promoted', 'active');
const draftActive = chunkLifecycleStatus('draft', 'active');
const approvedSuperseded = chunkLifecycleStatus('approved', 'superseded');
const promotedInactive = chunkLifecycleStatus('promoted', 'inactive');

function endpoint(
  ws: ReturnType<typeof workspaceId>,
  label: ReturnType<typeof ideaLabel>,
  status: ReturnType<typeof chunkLifecycleStatus>,
): RelationshipEndpointChunk {
  return { workspaceId: ws, label, status };
}

// ─── AC1: only approved/promoted ideas and active relationships are included ─

describe('AC1 — generated context contains only approved/promoted ideas and active relationships', () => {
  it('includes an approved+active chunk', () => {
    const item = includeChunkInGeneratedContext({
      workspaceId: ws1,
      label: ideaA,
      chunkType: 'feature',
      status: approvedActive,
    });
    expect(item.workspaceId).toBe(ws1);
    expect(item.provenance).toEqual({
      kind: 'chunk',
      sourceLabel: ideaA,
      chunkType: 'feature',
    });
  });

  it('includes a promoted+active chunk', () => {
    const item = includeChunkInGeneratedContext({
      workspaceId: ws1,
      label: ideaA,
      chunkType: 'adr',
      status: promotedActive,
    });
    expect(item.provenance).toMatchObject({ kind: 'chunk', sourceLabel: ideaA });
  });

  it('includes an active relationship whose endpoints are both approved/promoted+active', () => {
    const edge = createEdge(ws1, ideaA, ideaB, 'depends-on');
    const item = includeRelationshipInGeneratedContext({
      edge: edge.versions[0]!,
      sourceChunk: endpoint(ws1, ideaA, approvedActive),
      targetChunk: endpoint(ws1, ideaB, promotedActive),
    });
    expect(item.workspaceId).toBe(ws1);
    expect(item.provenance).toEqual({
      kind: 'relationship',
      sourceLabel: ideaA,
      targetLabel: ideaB,
      relationshipType: 'depends-on',
    });
  });

  it('createGeneratedContextPackage assembles included items into a package', () => {
    const chunkItem = includeChunkInGeneratedContext({
      workspaceId: ws1,
      label: ideaA,
      chunkType: 'feature',
      status: approvedActive,
    });
    const edgeItem = includeRelationshipInGeneratedContext({
      edge: createEdge(ws1, ideaA, ideaB, 'refines').versions[0]!,
      sourceChunk: endpoint(ws1, ideaA, approvedActive),
      targetChunk: endpoint(ws1, ideaB, promotedActive),
    });
    const pkg = createGeneratedContextPackage(
      generatedContextId('ctx-1'),
      ws1,
      'permanent',
      [chunkItem, edgeItem],
    );
    expect(pkg.items).toHaveLength(2);
    expect(pkg.contextKind).toBe('permanent');
    expect(pkg.workspaceId).toBe(ws1);
  });
});

// ─── AC2: each item can be traced back to its approved source ───────────────

describe('AC2 — each generated context item traces back to the idea or relationship that supports it', () => {
  it('a chunk item carries the exact source label', () => {
    const item = includeChunkInGeneratedContext({
      workspaceId: ws1,
      label: ideaA,
      chunkType: 'constraint',
      status: approvedActive,
    });
    expect(item.provenance.kind).toBe('chunk');
    expect((item.provenance as { sourceLabel: unknown }).sourceLabel).toBe(ideaA);
  });

  it('a relationship item carries both endpoint labels and the relationship type', () => {
    const edge = createEdge(ws1, ideaA, ideaB, 'implements');
    const item = includeRelationshipInGeneratedContext({
      edge: edge.versions[0]!,
      sourceChunk: endpoint(ws1, ideaA, approvedActive),
      targetChunk: endpoint(ws1, ideaB, approvedActive),
    });
    if (item.provenance.kind !== 'relationship') {
      expect.unreachable('expected relationship provenance');
    }
    expect(item.provenance.sourceLabel).toBe(ideaA);
    expect(item.provenance.targetLabel).toBe(ideaB);
    expect(item.provenance.relationshipType).toBe('implements');
  });
});

// ─── AC3: draft, superseded, inactive, and deactivated work is excluded ─────

describe('AC3 — draft, superseded, inactive, and deactivated work is not treated as approved source material', () => {
  it('rejects a draft chunk', () => {
    expect(() =>
      includeChunkInGeneratedContext({
        workspaceId: ws1,
        label: ideaA,
        chunkType: 'feature',
        status: draftActive,
      }),
    ).toThrow(GeneratedContextError);
  });

  it('rejects an approved+superseded chunk', () => {
    expect(() =>
      includeChunkInGeneratedContext({
        workspaceId: ws1,
        label: ideaA,
        chunkType: 'feature',
        status: approvedSuperseded,
      }),
    ).toThrow(GeneratedContextError);
  });

  it('rejects a promoted+inactive chunk', () => {
    expect(() =>
      includeChunkInGeneratedContext({
        workspaceId: ws1,
        label: ideaA,
        chunkType: 'feature',
        status: promotedInactive,
      }),
    ).toThrow(GeneratedContextError);
  });

  it('chunk rejection carries the invalid-state-transition code', () => {
    try {
      includeChunkInGeneratedContext({
        workspaceId: ws1,
        label: ideaA,
        chunkType: 'feature',
        status: draftActive,
      });
      expect.unreachable('expected includeChunkInGeneratedContext to throw');
    } catch (err) {
      expect(err).toBeInstanceOf(GeneratedContextError);
      expect((err as GeneratedContextError).code).toBe('invalid-state-transition');
    }
  });

  it('rejects a deactivated relationship', () => {
    const deactivated = deactivateEdge(createEdge(ws1, ideaA, ideaB, 'depends-on'));
    expect(() =>
      includeRelationshipInGeneratedContext({
        edge: deactivated.versions[deactivated.versions.length - 1]!,
        sourceChunk: endpoint(ws1, ideaA, approvedActive),
        targetChunk: endpoint(ws1, ideaB, approvedActive),
      }),
    ).toThrow(GeneratedContextError);
  });

  it('rejects a superseded relationship version', () => {
    const identity = {
      workspaceId: ws1,
      sourceLabel: ideaA,
      targetLabel: ideaB,
      relationshipType: 'depends-on' as const,
    };
    const superseded = supersedeEdge(createEdge(ws1, ideaA, ideaB, 'depends-on'), identity);
    // superseded.versions[0] is now state 'superseded'
    expect(() =>
      includeRelationshipInGeneratedContext({
        edge: superseded.versions[0]!,
        sourceChunk: endpoint(ws1, ideaA, approvedActive),
        targetChunk: endpoint(ws1, ideaB, approvedActive),
      }),
    ).toThrow(GeneratedContextError);
  });

  it('rejects an active relationship whose source chunk is draft', () => {
    const edge = createEdge(ws1, ideaA, ideaB, 'depends-on');
    expect(() =>
      includeRelationshipInGeneratedContext({
        edge: edge.versions[0]!,
        sourceChunk: endpoint(ws1, ideaA, draftActive),
        targetChunk: endpoint(ws1, ideaB, approvedActive),
      }),
    ).toThrow(GeneratedContextError);
  });

  it('rejects an active relationship whose target chunk is superseded', () => {
    const edge = createEdge(ws1, ideaA, ideaB, 'depends-on');
    expect(() =>
      includeRelationshipInGeneratedContext({
        edge: edge.versions[0]!,
        sourceChunk: endpoint(ws1, ideaA, approvedActive),
        targetChunk: endpoint(ws1, ideaB, approvedSuperseded),
      }),
    ).toThrow(GeneratedContextError);
  });

  it('relationship rejection carries the invalid-state-transition code', () => {
    const deactivated = deactivateEdge(createEdge(ws1, ideaA, ideaB, 'depends-on'));
    try {
      includeRelationshipInGeneratedContext({
        edge: deactivated.versions[deactivated.versions.length - 1]!,
        sourceChunk: endpoint(ws1, ideaA, approvedActive),
        targetChunk: endpoint(ws1, ideaB, approvedActive),
      });
      expect.unreachable('expected includeRelationshipInGeneratedContext to throw');
    } catch (err) {
      expect(err).toBeInstanceOf(GeneratedContextError);
      expect((err as GeneratedContextError).code).toBe('invalid-state-transition');
    }
  });

  it('rejects a relationship whose source endpoint label does not match the edge', () => {
    const edge = createEdge(ws1, ideaA, ideaB, 'depends-on');
    expect(() =>
      includeRelationshipInGeneratedContext({
        edge: edge.versions[0]!,
        // wrong label: caller mismatched a status meant for a different idea
        sourceChunk: endpoint(ws1, ideaC, approvedActive),
        targetChunk: endpoint(ws1, ideaB, approvedActive),
      }),
    ).toThrow(GeneratedContextError);
  });

  it('rejects a relationship whose endpoint chunk is from a different workspace', () => {
    const edge = createEdge(ws1, ideaA, ideaB, 'depends-on');
    try {
      includeRelationshipInGeneratedContext({
        edge: edge.versions[0]!,
        sourceChunk: endpoint(ws2, ideaA, approvedActive),
        targetChunk: endpoint(ws1, ideaB, approvedActive),
      });
      expect.unreachable('expected includeRelationshipInGeneratedContext to throw');
    } catch (err) {
      expect(err).toBeInstanceOf(GeneratedContextError);
      expect((err as GeneratedContextError).code).toBe('tenant-boundary-violation');
    }
  });

  it('createGeneratedContextPackage rejects an item from a different workspace (tenant boundary)', () => {
    const foreignItem = includeChunkInGeneratedContext({
      workspaceId: ws2,
      label: ideaA,
      chunkType: 'feature',
      status: approvedActive,
    });
    expect(() =>
      createGeneratedContextPackage(generatedContextId('ctx-2'), ws1, 'permanent', [
        foreignItem,
      ]),
    ).toThrow(GeneratedContextError);
  });

  it('tenant boundary rejection carries the tenant-boundary-violation code', () => {
    const foreignItem = includeChunkInGeneratedContext({
      workspaceId: ws2,
      label: ideaA,
      chunkType: 'feature',
      status: approvedActive,
    });
    try {
      createGeneratedContextPackage(generatedContextId('ctx-3'), ws1, 'permanent', [
        foreignItem,
      ]);
      expect.unreachable('expected createGeneratedContextPackage to throw');
    } catch (err) {
      expect(err).toBeInstanceOf(GeneratedContextError);
      expect((err as GeneratedContextError).code).toBe('tenant-boundary-violation');
    }
  });

  it('createGeneratedContextPackage rejects duplicate active relationships for the same triple', () => {
    const edgeItem1 = includeRelationshipInGeneratedContext({
      edge: createEdge(ws1, ideaA, ideaB, 'depends-on').versions[0]!,
      sourceChunk: endpoint(ws1, ideaA, approvedActive),
      targetChunk: endpoint(ws1, ideaB, approvedActive),
    });
    const edgeItem2 = includeRelationshipInGeneratedContext({
      edge: createEdge(ws1, ideaA, ideaB, 'depends-on').versions[0]!,
      sourceChunk: endpoint(ws1, ideaA, approvedActive),
      targetChunk: endpoint(ws1, ideaB, approvedActive),
    });
    expect(() =>
      createGeneratedContextPackage(generatedContextId('ctx-4'), ws1, 'permanent', [
        edgeItem1,
        edgeItem2,
      ]),
    ).toThrow(GeneratedContextError);
  });

  it('duplicate relationship rejection carries the duplicate-active-relationship code', () => {
    const edgeItem1 = includeRelationshipInGeneratedContext({
      edge: createEdge(ws1, ideaA, ideaB, 'depends-on').versions[0]!,
      sourceChunk: endpoint(ws1, ideaA, approvedActive),
      targetChunk: endpoint(ws1, ideaB, approvedActive),
    });
    const edgeItem2 = includeRelationshipInGeneratedContext({
      edge: createEdge(ws1, ideaA, ideaB, 'depends-on').versions[0]!,
      sourceChunk: endpoint(ws1, ideaA, approvedActive),
      targetChunk: endpoint(ws1, ideaB, approvedActive),
    });
    try {
      createGeneratedContextPackage(generatedContextId('ctx-5'), ws1, 'permanent', [
        edgeItem1,
        edgeItem2,
      ]);
      expect.unreachable('expected createGeneratedContextPackage to throw');
    } catch (err) {
      expect(err).toBeInstanceOf(GeneratedContextError);
      expect((err as GeneratedContextError).code).toBe('duplicate-active-relationship');
    }
  });

  it('does not throw for distinct relationship triples in the same package', () => {
    const edgeItem1 = includeRelationshipInGeneratedContext({
      edge: createEdge(ws1, ideaA, ideaB, 'depends-on').versions[0]!,
      sourceChunk: endpoint(ws1, ideaA, approvedActive),
      targetChunk: endpoint(ws1, ideaB, approvedActive),
    });
    const edgeItem2 = includeRelationshipInGeneratedContext({
      edge: createEdge(ws1, ideaB, ideaC, 'refines').versions[0]!,
      sourceChunk: endpoint(ws1, ideaB, approvedActive),
      targetChunk: endpoint(ws1, ideaC, approvedActive),
    });
    expect(() =>
      createGeneratedContextPackage(generatedContextId('ctx-6'), ws1, 'permanent', [
        edgeItem1,
        edgeItem2,
      ]),
    ).not.toThrow();
  });

  it('createGeneratedContextPackage rejects a structurally forged item that never passed through an inclusion function', () => {
    // Compile-time branding (`[_itemBrand]: never`) is erased at runtime, so
    // this simulates a caller/deserializer fabricating an object that merely
    // looks like a GeneratedContextItem without going through
    // includeChunkInGeneratedContext / includeRelationshipInGeneratedContext.
    const forged = {
      workspaceId: ws1,
      provenance: { kind: 'chunk', sourceLabel: ideaA, chunkType: 'feature' },
    } as unknown as GeneratedContextItem;
    try {
      createGeneratedContextPackage(generatedContextId('ctx-forged'), ws1, 'permanent', [
        forged,
      ]);
      expect.unreachable('expected createGeneratedContextPackage to throw');
    } catch (err) {
      expect(err).toBeInstanceOf(GeneratedContextError);
      expect((err as GeneratedContextError).code).toBe('invalid-state-transition');
    }
  });
});

// ─── AC4: generated context is a read-only projection, not the source of truth ─

describe('AC4 — generated context is a projection, not the source of truth', () => {
  it('an empty item list produces a valid (empty) package', () => {
    const pkg = createGeneratedContextPackage(
      generatedContextId('ctx-empty'),
      ws1,
      'transient',
      [],
    );
    expect(pkg.items).toHaveLength(0);
  });

  it('the package is frozen', () => {
    const pkg = createGeneratedContextPackage(generatedContextId('ctx-7'), ws1, 'permanent', []);
    expect(Object.isFrozen(pkg)).toBe(true);
  });

  it('the items array is frozen', () => {
    const item = includeChunkInGeneratedContext({
      workspaceId: ws1,
      label: ideaA,
      chunkType: 'feature',
      status: approvedActive,
    });
    const pkg = createGeneratedContextPackage(generatedContextId('ctx-8'), ws1, 'permanent', [
      item,
    ]);
    expect(Object.isFrozen(pkg.items)).toBe(true);
  });

  it('each item in the package is frozen', () => {
    const item = includeChunkInGeneratedContext({
      workspaceId: ws1,
      label: ideaA,
      chunkType: 'feature',
      status: approvedActive,
    });
    const pkg = createGeneratedContextPackage(generatedContextId('ctx-9'), ws1, 'permanent', [
      item,
    ]);
    expect(Object.isFrozen(pkg.items[0])).toBe(true);
    expect(Object.isFrozen(pkg.items[0]!.provenance)).toBe(true);
  });

  it('an item produced by includeChunkInGeneratedContext is itself frozen before packaging', () => {
    const item = includeChunkInGeneratedContext({
      workspaceId: ws1,
      label: ideaA,
      chunkType: 'feature',
      status: approvedActive,
    });
    expect(Object.isFrozen(item)).toBe(true);
    expect(Object.isFrozen(item.provenance)).toBe(true);
  });

  it('the package does not alias the caller-supplied items array', () => {
    const item = includeChunkInGeneratedContext({
      workspaceId: ws1,
      label: ideaA,
      chunkType: 'feature',
      status: approvedActive,
    });
    const callerItems: GeneratedContextItem[] = [item];
    const pkg = createGeneratedContextPackage(
      generatedContextId('ctx-10'),
      ws1,
      'permanent',
      callerItems,
    );
    expect(pkg.items).not.toBe(callerItems);
  });
});

// ─── AC5: generated context preserves the business meaning of its sources ──

describe('AC5 — generated context preserves the business meaning of the ideas it came from', () => {
  it('preserves the exact chunk type through inclusion', () => {
    const item = includeChunkInGeneratedContext({
      workspaceId: ws1,
      label: ideaA,
      chunkType: 'spike',
      status: approvedActive,
    });
    if (item.provenance.kind !== 'chunk') {
      expect.unreachable('expected chunk provenance');
    }
    expect(item.provenance.chunkType).toBe('spike');
  });

  it('preserves the exact relationship type and both idea label endpoints through inclusion', () => {
    const item = includeRelationshipInGeneratedContext({
      edge: createEdge(ws1, ideaA, ideaB, 'supersedes').versions[0]!,
      sourceChunk: endpoint(ws1, ideaA, approvedActive),
      targetChunk: endpoint(ws1, ideaB, promotedActive),
    });
    if (item.provenance.kind !== 'relationship') {
      expect.unreachable('expected relationship provenance');
    }
    expect(item.provenance.sourceLabel).toBe(ideaA);
    expect(item.provenance.targetLabel).toBe(ideaB);
    expect(item.provenance.relationshipType).toBe('supersedes');
  });

  it('a package assembled from multiple sources preserves each item distinctly', () => {
    const chunkItem = includeChunkInGeneratedContext({
      workspaceId: ws1,
      label: ideaA,
      chunkType: 'capability',
      status: promotedActive,
    });
    const edgeItem = includeRelationshipInGeneratedContext({
      edge: createEdge(ws1, ideaA, ideaB, 'informs').versions[0]!,
      sourceChunk: endpoint(ws1, ideaA, promotedActive),
      targetChunk: endpoint(ws1, ideaB, approvedActive),
    });
    const pkg = createGeneratedContextPackage(
      generatedContextId('ctx-11'),
      ws1,
      'permanent',
      [chunkItem, edgeItem],
    );
    expect(pkg.items.map((i) => i.provenance)).toEqual([
      { kind: 'chunk', sourceLabel: ideaA, chunkType: 'capability' },
      { kind: 'relationship', sourceLabel: ideaA, targetLabel: ideaB, relationshipType: 'informs' },
    ]);
  });
});
