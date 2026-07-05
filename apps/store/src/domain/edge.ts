import { randomUUID } from 'node:crypto';
import type { Discipline } from './types/vocabulary/discipline.js';
import { parseDiscipline } from './types/vocabulary/discipline.js';
import type { EdgeStatus } from './types/vocabulary/edge-status.js';
import { parseEdgeStatus } from './types/vocabulary/edge-status.js';
import type { EdgeType } from './types/vocabulary/edge-type.js';
import { parseEdgeType } from './types/vocabulary/edge-type.js';

export interface EdgeProps {
  id?: string;
  fromChunkLabel: string;
  toChunkLabel: string;
  type: EdgeType;
  status?: EdgeStatus;
  discipline: Discipline;
  branchId?: string;
  originBranchId?: string;
  supersededByEdgeId?: string;
  createdByStakeholderId: string;
  updatedByStakeholderId?: string;
  createdAt?: Date;
  updatedAt?: Date;
}

function requireNonBlank(value: string, fieldName: string): string {
  if (value.trim().length === 0) {
    throw new TypeError(`Edge ${fieldName} must not be empty or blank`);
  }
  return value;
}

/**
 * Edge entity: a typed, directed relationship between two chunks, referenced by their logical
 * string labels rather than database ids (Meridian IDEA-36/IDEA-37), so relationships remain
 * valid across branch overrides and mainline promotions. Mainline edges are immutable and can
 * only be modified/deactivated by superseding with a new edge version chained via
 * supersededByEdgeId (Meridian IDEA-38); this goal's write path only ever produces 'active'
 * edges with supersededByEdgeId left undefined.
 */
export class Edge {
  readonly id: string;
  readonly fromChunkLabel: string;
  readonly toChunkLabel: string;
  readonly type: EdgeType;
  readonly status: EdgeStatus;
  readonly discipline: Discipline;
  readonly branchId: string | undefined;
  readonly originBranchId: string | undefined;
  readonly supersededByEdgeId: string | undefined;
  readonly createdByStakeholderId: string;
  readonly updatedByStakeholderId: string;
  readonly createdAt: Date;
  readonly updatedAt: Date;

  constructor(props: EdgeProps) {
    this.fromChunkLabel = requireNonBlank(props.fromChunkLabel, 'fromChunkLabel');
    this.toChunkLabel = requireNonBlank(props.toChunkLabel, 'toChunkLabel');

    if (this.fromChunkLabel === this.toChunkLabel) {
      throw new TypeError('Edge fromChunkLabel and toChunkLabel must not be the same label');
    }

    this.type = parseEdgeType(props.type);
    this.status = parseEdgeStatus(props.status ?? 'active');
    this.discipline = parseDiscipline(props.discipline);

    if (props.createdByStakeholderId.trim().length === 0) {
      throw new TypeError('Edge requires a non-blank createdByStakeholderId');
    }

    this.id = props.id ?? randomUUID();
    this.branchId = props.branchId;
    this.originBranchId = props.originBranchId;
    this.supersededByEdgeId = props.supersededByEdgeId;
    this.createdByStakeholderId = props.createdByStakeholderId;
    this.updatedByStakeholderId = props.updatedByStakeholderId ?? props.createdByStakeholderId;
    this.createdAt = props.createdAt ?? new Date();
    this.updatedAt = props.updatedAt ?? this.createdAt;
  }
}
