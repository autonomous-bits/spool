import { randomUUID } from 'node:crypto';
import type { ChunkType } from './types/vocabulary/chunk-type.js';
import { parseChunkType } from './types/vocabulary/chunk-type.js';
import type { ContextKind } from './types/vocabulary/context-kind.js';
import { parseContextKind } from './types/vocabulary/context-kind.js';
import type { Discipline } from './types/vocabulary/discipline.js';
import { parseDiscipline } from './types/vocabulary/discipline.js';

/**
 * Chunk lifecycle status. G01 only ever produces 'draft' chunks (branchless capture, ratified by
 * Meridian IDEA-78); 'promoted' and later states are governed by later goals.
 */
export type ChunkStatus = 'draft' | 'promoted' | 'superseded' | 'deactivated';

export interface ChunkProps {
  id?: string;
  label: string;
  content: string;
  discipline: Discipline;
  chunkType: ChunkType;
  contextKind: ContextKind;
  createdByStakeholderId: string;
  updatedByStakeholderId?: string;
  status?: ChunkStatus;
  branchId?: string;
  originBranchId?: string;
  createdAt?: Date;
  updatedAt?: Date;
}

function requireNonBlank(value: string, fieldName: string): string {
  if (value.trim().length === 0) {
    throw new TypeError(`Chunk ${fieldName} must not be empty or blank`);
  }
  return value;
}

/**
 * Chunk entity: an atomic idea chunk, captured either as a branchless mainline draft
 * (branchId/originBranchId undefined, per Meridian IDEA-78) or attached to an existing draft
 * branch (branchId/originBranchId both set to that branch's id, per G02/IDEA-40). Enforces the
 * capture invariants ratified for G01: non-blank label/content, a valid closed vocabulary for
 * discipline/chunkType/contextKind, and a required authoring stakeholder (Meridian IDEA-11: every
 * chunk is attributed to a stakeholder).
 */
export class Chunk {
  readonly id: string;
  readonly label: string;
  readonly content: string;
  readonly discipline: Discipline;
  readonly chunkType: ChunkType;
  readonly contextKind: ContextKind;
  readonly status: ChunkStatus;
  readonly createdByStakeholderId: string;
  readonly updatedByStakeholderId: string;
  readonly branchId: string | undefined;
  readonly originBranchId: string | undefined;
  readonly createdAt: Date;
  readonly updatedAt: Date;

  constructor(props: ChunkProps) {
    this.label = requireNonBlank(props.label, 'label');
    this.content = requireNonBlank(props.content, 'content');
    this.discipline = parseDiscipline(props.discipline);
    this.chunkType = parseChunkType(props.chunkType);
    this.contextKind = parseContextKind(props.contextKind);

    if (props.createdByStakeholderId.trim().length === 0) {
      throw new TypeError('Chunk requires a non-blank createdByStakeholderId');
    }

    this.id = props.id ?? randomUUID();
    this.createdByStakeholderId = props.createdByStakeholderId;
    this.updatedByStakeholderId = props.updatedByStakeholderId ?? props.createdByStakeholderId;
    this.status = props.status ?? 'draft';
    this.branchId = props.branchId;
    this.originBranchId = props.originBranchId;
    this.createdAt = props.createdAt ?? new Date();
    this.updatedAt = props.updatedAt ?? this.createdAt;
  }
}
