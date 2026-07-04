/**
 * Generated context domain module: provenance-carrying projections built
 * exclusively from approved or promoted idea chunks and their active,
 * label-based relationships.
 *
 * Sources of authority:
 * - Story:          docs/specifications/feature-01-core-domain-model/stories/S08-traceable-generated-context.md
 * - Functional spec: docs/specifications/feature-01-core-domain-model/functional-specification.md
 *                    outcome 6, AC5
 * - Technical spec: docs/specifications/feature-01-core-domain-model/technical-specification.md
 *                   §"Generated context", §"Workspace scoping", §"Edge determinism",
 *                   §"Required domain error categories"
 * - Constitution:   docs/constitution.md (Principle IV — Rich Domain Models)
 * - Meridian:       IDEA-36, IDEA-37, IDEA-38
 *
 * Story: S08 — Give agents traceable approved implementation context.
 *
 * Design notes:
 * - This module does not re-derive lifecycle or edge-state logic. Eligibility
 *   is delegated entirely to the existing `isSafeForImplementationUse`
 *   (chunk-lifecycle.ts) and `isActiveEdge` (edge-lineage.ts) predicates, so
 *   there is exactly one place in the codebase that decides "is this chunk /
 *   edge safe to use".
 * - `GeneratedContextItem` is an opaque, branded type constructible only
 *   through `includeChunkInGeneratedContext` and
 *   `includeRelationshipInGeneratedContext`. This mirrors the `EdgeLineage`
 *   pattern in `edge-lineage.ts` and closes off any public path that could
 *   assemble a `GeneratedContextPackage` from forged/unvalidated items —
 *   the type system, not caller discipline, enforces AC1/AC3.
 * - A relationship is only includable when the edge itself is active *and*
 *   both of its endpoint chunks are independently safe for implementation
 *   use. An active edge between a draft or superseded chunk is not "approved
 *   source material" (technical spec §"Generated context"; vocabulary.md
 *   "Generated context" section).
 * - `GeneratedContextPackage` is a read-only projection (AC4): construction
 *   deep-freezes the package, its items, and every provenance object, and
 *   never aliases caller-supplied arrays/objects. There is no public mutation
 *   API — a `GeneratedContextPackage` cannot be written back into the graph.
 */

import type {
  ChunkType,
  ContextKind,
  GeneratedContextId,
  IdeaLabel,
  RelationshipType,
  WorkspaceId,
} from './types/index.js';
import { GeneratedContextError } from './types/index.js';
import type { ChunkLifecycleStatus } from './chunk-lifecycle.js';
import { isSafeForImplementationUse } from './chunk-lifecycle.js';
import type { RelationshipEdgeVersion } from './edge-lineage.js';
import { isActiveEdge } from './edge-lineage.js';

export { GeneratedContextError };
export type { GeneratedContextErrorCode } from './types/index.js';

/**
 * Provenance for a generated context item sourced directly from an idea
 * chunk.
 *
 * AC2: "A stakeholder can trace each generated context item back to the
 * approved idea ... that supports it."
 */
export interface ChunkProvenance {
  readonly kind: 'chunk';
  readonly sourceLabel: IdeaLabel;
  readonly chunkType: ChunkType;
}

/**
 * Provenance for a generated context item sourced from an active,
 * label-based relationship between two idea chunks.
 *
 * AC2: "... or relationship that supports it." Endpoints are `IdeaLabel`
 * values, never storage-row identifiers (technical spec §"Logical edge
 * endpoints"; Meridian IDEA-36, IDEA-37).
 */
export interface RelationshipProvenance {
  readonly kind: 'relationship';
  readonly sourceLabel: IdeaLabel;
  readonly targetLabel: IdeaLabel;
  readonly relationshipType: RelationshipType;
}

export type ContextProvenance = ChunkProvenance | RelationshipProvenance;

const _itemBrand: unique symbol = Symbol('GeneratedContextItem');
const _packageBrand: unique symbol = Symbol('GeneratedContextPackage');

/**
 * A single provenance-carrying item in a generated context package.
 *
 * Opaque and frozen. Constructible only via `includeChunkInGeneratedContext`
 * and `includeRelationshipInGeneratedContext`, both of which enforce the
 * "approved or promoted, active" eligibility rule before an item can exist.
 */
export type GeneratedContextItem = {
  readonly workspaceId: WorkspaceId;
  readonly provenance: ContextProvenance;
  readonly [_itemBrand]: never;
};

/**
 * A workspace-scoped, read-only projection of generated context items.
 *
 * AC4: "... generated documents are projections from approved knowledge, not
 * the source of truth." There is no mutation API: a package is built once,
 * via `createGeneratedContextPackage`, from already-validated items, and is
 * deep-frozen on construction.
 */
export type GeneratedContextPackage = {
  readonly id: GeneratedContextId;
  readonly workspaceId: WorkspaceId;
  readonly contextKind: ContextKind;
  readonly items: readonly GeneratedContextItem[];
  readonly [_packageBrand]: never;
};

/**
 * Runtime authenticity registry for `GeneratedContextItem`s.
 *
 * TypeScript's `[_itemBrand]: never` marker is erased at runtime, so it
 * cannot by itself stop a caller from structurally fabricating an object
 * that merely looks like a `GeneratedContextItem` and feeding it to
 * `createGeneratedContextPackage`. This `WeakSet` records every item object
 * actually produced by `includeChunkInGeneratedContext` or
 * `includeRelationshipInGeneratedContext` — the only two functions that
 * enforce the "approved/promoted, active" eligibility rule — and
 * `createGeneratedContextPackage` rejects any item that is not a member,
 * closing the forgery path that pure compile-time branding leaves open.
 */
const validatedItems = new WeakSet<object>();

function freezeProvenance(provenance: ContextProvenance): ContextProvenance {
  return Object.freeze({ ...provenance });
}

function freezeItem(item: {
  workspaceId: WorkspaceId;
  provenance: ContextProvenance;
}): GeneratedContextItem {
  const frozen = Object.freeze({
    workspaceId: item.workspaceId,
    provenance: freezeProvenance(item.provenance),
  }) as GeneratedContextItem;
  validatedItems.add(frozen);
  return frozen;
}

/**
 * Includes an idea chunk in generated context as a `GeneratedContextItem`,
 * carrying its provenance back to the source idea label.
 *
 * AC1/AC3: only chunks that are safe for implementation use (approved or
 * promoted, and active — see `isSafeForImplementationUse`) may be included.
 * Draft, superseded, and inactive chunks are rejected.
 *
 * Throws `GeneratedContextError` with code `invalid-state-transition` if
 * `status` is not safe for implementation use.
 */
export function includeChunkInGeneratedContext(input: {
  readonly workspaceId: WorkspaceId;
  readonly label: IdeaLabel;
  readonly chunkType: ChunkType;
  readonly status: ChunkLifecycleStatus;
}): GeneratedContextItem {
  if (!isSafeForImplementationUse(input.status)) {
    throw new GeneratedContextError(
      'invalid-state-transition',
      `idea '${input.label}' is not approved or promoted and active; ` +
        'draft, superseded, and inactive chunks cannot be included in generated context',
    );
  }
  return freezeItem({
    workspaceId: input.workspaceId,
    provenance: {
      kind: 'chunk',
      sourceLabel: input.label,
      chunkType: input.chunkType,
    },
  });
}

/**
 * Identifies the workspace-scoped idea chunk at one end of a relationship
 * edge, together with its current lifecycle status.
 *
 * `includeRelationshipInGeneratedContext` cross-checks `workspaceId` and
 * `label` against the edge's own identity so that endpoint statuses cannot be
 * supplied for an unrelated chunk or a chunk in a different workspace.
 */
export interface RelationshipEndpointChunk {
  readonly workspaceId: WorkspaceId;
  readonly label: IdeaLabel;
  readonly status: ChunkLifecycleStatus;
}

function assertEndpointMatchesEdge(
  endpointKind: 'source' | 'target',
  endpoint: RelationshipEndpointChunk,
  edge: RelationshipEdgeVersion,
  expectedLabel: IdeaLabel,
): void {
  if (endpoint.workspaceId !== edge.workspaceId) {
    throw new GeneratedContextError(
      'tenant-boundary-violation',
      `relationship ${endpointKind} chunk '${endpoint.label}' belongs to workspace ` +
        `'${endpoint.workspaceId}', but the relationship is scoped to workspace '${edge.workspaceId}'`,
    );
  }
  if (endpoint.label !== expectedLabel) {
    throw new GeneratedContextError(
      'invalid-state-transition',
      `relationship ${endpointKind} chunk status was supplied for idea '${endpoint.label}', ` +
        `but the relationship's ${endpointKind} label is '${expectedLabel}'`,
    );
  }
}

/**
 * Includes a relationship in generated context as a `GeneratedContextItem`,
 * carrying its provenance back to the source and target idea labels and the
 * relationship type.
 *
 * AC1/AC3: the relationship edge itself must be active (`isActiveEdge`), and
 * — because an active edge between unsafe chunks is not "approved source
 * material" — both endpoint chunks must independently be safe for
 * implementation use. A relationship whose edge is active but whose source or
 * target chunk is draft, superseded, or inactive is rejected. Each endpoint
 * chunk must also carry the same workspace and idea label as the edge itself
 * (`assertEndpointMatchesEdge`), so a caller cannot satisfy the eligibility
 * check with an unrelated chunk's status.
 *
 * Throws `GeneratedContextError` with code `invalid-state-transition` if the
 * edge is not active, if either endpoint chunk is not safe for implementation
 * use, or if an endpoint's idea label does not match the edge's
 * corresponding label.
 * Throws `GeneratedContextError` with code `tenant-boundary-violation` if an
 * endpoint chunk's workspace does not match the edge's workspace.
 */
export function includeRelationshipInGeneratedContext(input: {
  readonly edge: RelationshipEdgeVersion;
  readonly sourceChunk: RelationshipEndpointChunk;
  readonly targetChunk: RelationshipEndpointChunk;
}): GeneratedContextItem {
  const { edge, sourceChunk, targetChunk } = input;
  if (!isActiveEdge(edge)) {
    throw new GeneratedContextError(
      'invalid-state-transition',
      `relationship '${edge.sourceLabel}' -[${edge.relationshipType}]-> '${edge.targetLabel}' ` +
        `is '${edge.state}', not active; only active relationships can be included in generated context`,
    );
  }
  assertEndpointMatchesEdge('source', sourceChunk, edge, edge.sourceLabel);
  assertEndpointMatchesEdge('target', targetChunk, edge, edge.targetLabel);
  if (!isSafeForImplementationUse(sourceChunk.status)) {
    throw new GeneratedContextError(
      'invalid-state-transition',
      `relationship source idea '${edge.sourceLabel}' is not approved or promoted and active; ` +
        'an active relationship endpoint must itself be approved source material',
    );
  }
  if (!isSafeForImplementationUse(targetChunk.status)) {
    throw new GeneratedContextError(
      'invalid-state-transition',
      `relationship target idea '${edge.targetLabel}' is not approved or promoted and active; ` +
        'an active relationship endpoint must itself be approved source material',
    );
  }
  return freezeItem({
    workspaceId: edge.workspaceId,
    provenance: {
      kind: 'relationship',
      sourceLabel: edge.sourceLabel,
      targetLabel: edge.targetLabel,
      relationshipType: edge.relationshipType,
    },
  });
}

function relationshipTripleKey(provenance: RelationshipProvenance): string {
  return `${provenance.sourceLabel}\u0000${provenance.targetLabel}\u0000${provenance.relationshipType}`;
}

/**
 * Assembles a set of already-validated `GeneratedContextItem`s into a frozen,
 * workspace-scoped `GeneratedContextPackage`.
 *
 * AC1: every item must belong to the package's workspace (technical spec
 * §"Workspace scoping": generated context "must not connect or resolve"
 * across workspaces).
 *
 * Technical spec §"Edge determinism": a resolved view may contain at most one
 * active edge for the same source label, target label, and relationship
 * type — this constraint carries over to a generated context package.
 *
 * AC4: the returned package (and every item and provenance object within it)
 * is deep-frozen and never aliases the caller-supplied `items` array — the
 * package is a read-only projection, not a mutable working set.
 *
 * Every item must have been produced by `includeChunkInGeneratedContext` or
 * `includeRelationshipInGeneratedContext` (tracked in the `validatedItems`
 * registry); this closes the forgery path that compile-time branding alone
 * cannot stop, since `[_itemBrand]: never` is erased at runtime.
 *
 * Throws `GeneratedContextError` with code `invalid-state-transition` if any
 * item was not produced by `includeChunkInGeneratedContext` or
 * `includeRelationshipInGeneratedContext`.
 * Throws `GeneratedContextError` with code `tenant-boundary-violation` if any
 * item's `workspaceId` does not match `workspaceId`.
 * Throws `GeneratedContextError` with code `duplicate-active-relationship` if
 * two relationship items share the same source label, target label, and
 * relationship type.
 */
export function createGeneratedContextPackage(
  id: GeneratedContextId,
  workspaceId: WorkspaceId,
  contextKind: ContextKind,
  items: readonly GeneratedContextItem[],
): GeneratedContextPackage {
  const seenTriples = new Set<string>();
  for (const item of items) {
    if (!validatedItems.has(item)) {
      throw new GeneratedContextError(
        'invalid-state-transition',
        'a generated context package can only contain items produced by ' +
          'includeChunkInGeneratedContext or includeRelationshipInGeneratedContext',
      );
    }
    if (item.workspaceId !== workspaceId) {
      throw new GeneratedContextError(
        'tenant-boundary-violation',
        `a generated context package must be scoped to a single workspace; ` +
          `found item from '${item.workspaceId}' in a package for '${workspaceId}'`,
      );
    }
    if (item.provenance.kind === 'relationship') {
      const key = relationshipTripleKey(item.provenance);
      if (seenTriples.has(key)) {
        throw new GeneratedContextError(
          'duplicate-active-relationship',
          `a generated context package cannot contain more than one active relationship for ` +
            `'${item.provenance.sourceLabel}' -[${item.provenance.relationshipType}]-> '${item.provenance.targetLabel}'`,
        );
      }
      seenTriples.add(key);
    }
  }

  return Object.freeze({
    id,
    workspaceId,
    contextKind,
    items: Object.freeze(
      items.map((item) =>
        Object.freeze({
          workspaceId: item.workspaceId,
          provenance: freezeProvenance(item.provenance),
        }),
      ),
    ) as readonly GeneratedContextItem[],
  }) as GeneratedContextPackage;
}
