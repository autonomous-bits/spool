import { randomBytes, randomUUID } from 'node:crypto';
import type { Discipline } from './types/vocabulary/discipline.js';
import { isDiscipline } from './types/vocabulary/discipline.js';

const SIGNING_SECRET_BYTES = 32;

export class InvalidDeliverySubscriptionError extends Error {
  constructor(reason: string) {
    super(`Invalid DeliverySubscription: ${reason}`);
    this.name = 'InvalidDeliverySubscriptionError';
  }
}

export interface DeliverySubscriptionProps {
  id?: string;
  workspaceId: string;
  url: string;
  disciplineFilter?: readonly Discipline[];
  signingSecret?: string;
  isActive?: boolean;
  createdByStakeholderId: string;
  createdAt?: Date;
  updatedAt?: Date;
}

function requireNonBlank(value: string, fieldName: string): string {
  if (value.trim().length === 0) {
    throw new InvalidDeliverySubscriptionError(`${fieldName} must not be empty or blank`);
  }
  return value;
}

function requireHttpsUrl(url: string): string {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    throw new InvalidDeliverySubscriptionError(`url must be a valid URL: ${JSON.stringify(url)}`);
  }

  if (parsed.protocol !== 'https:') {
    throw new InvalidDeliverySubscriptionError(`url must use https://, got: ${JSON.stringify(url)}`);
  }

  return url;
}

function validateDisciplineFilter(
  disciplineFilter: readonly Discipline[] | undefined,
): readonly Discipline[] | undefined {
  if (disciplineFilter === undefined) {
    return undefined;
  }

  for (const value of disciplineFilter) {
    if (!isDiscipline(value)) {
      throw new InvalidDeliverySubscriptionError(`disciplineFilter contains invalid value: ${JSON.stringify(value)}`);
    }
  }

  return disciplineFilter;
}

function generateSigningSecret(): string {
  return randomBytes(SIGNING_SECRET_BYTES).toString('base64url');
}

/**
 * DeliverySubscription entity: a workspace-scoped webhook registration for the (deferred)
 * downstream delivery worker (Meridian IDEA-63/IDEA-65/IDEA-104). Registration invariants only —
 * the worker's trigger/payload/delivery-attempt behavior is out of scope (IDEA-126, G13 OQ1).
 *
 * `url` must be `https://` (rejecting both malformed URLs and plain `http://`); `disciplineFilter`
 * omission means "all disciplines"; `signingSecret` is always generated server-side via
 * `node:crypto`, never caller-supplied, so a consumer's shared secret can never be chosen or
 * observed by anyone other than the store at registration time.
 */
export class DeliverySubscription {
  readonly id: string;
  readonly workspaceId: string;
  readonly url: string;
  readonly disciplineFilter: readonly Discipline[] | undefined;
  readonly signingSecret: string;
  readonly isActive: boolean;
  readonly createdByStakeholderId: string;
  readonly createdAt: Date;
  readonly updatedAt: Date;

  constructor(props: DeliverySubscriptionProps) {
    this.workspaceId = requireNonBlank(props.workspaceId, 'workspaceId');
    this.url = requireHttpsUrl(props.url);
    this.disciplineFilter = validateDisciplineFilter(props.disciplineFilter);

    if (props.createdByStakeholderId.trim().length === 0) {
      throw new InvalidDeliverySubscriptionError('createdByStakeholderId must not be empty or blank');
    }

    this.id = props.id ?? randomUUID();
    this.signingSecret = props.signingSecret ?? generateSigningSecret();
    this.isActive = props.isActive ?? true;
    this.createdByStakeholderId = props.createdByStakeholderId;
    this.createdAt = props.createdAt ?? new Date();
    this.updatedAt = props.updatedAt ?? this.createdAt;
  }
}
