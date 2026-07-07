import { randomUUID } from 'node:crypto';

export interface WorkspaceProps {
  id?: string;
  name: string;
  createdByStakeholderId: string;
  createdAt?: Date;
  updatedAt?: Date;
}

function requireNonBlank(value: string, fieldName: string): string {
  if (value.trim().length === 0) {
    throw new TypeError(`Workspace ${fieldName} must not be empty or blank`);
  }
  return value;
}

/**
 * Workspace entity: a project/product-line scope with its own isolated idea-chunk graph, per
 * Meridian IDEA-89. This goal establishes the registry (Workspace + WorkspaceMembership) and
 * direct-add membership bootstrap (IDEA-88/94/95); enforcing graph isolation on existing tables
 * (IDEA-90) is explicitly deferred to a future goal.
 */
export class Workspace {
  readonly id: string;
  readonly name: string;
  readonly createdByStakeholderId: string;
  readonly createdAt: Date;
  readonly updatedAt: Date;

  constructor(props: WorkspaceProps) {
    this.name = requireNonBlank(props.name, 'name');

    if (props.createdByStakeholderId.trim().length === 0) {
      throw new TypeError('Workspace requires a non-blank createdByStakeholderId');
    }

    this.id = props.id ?? randomUUID();
    this.createdByStakeholderId = props.createdByStakeholderId;
    this.createdAt = props.createdAt ?? new Date();
    this.updatedAt = props.updatedAt ?? this.createdAt;
  }
}
