import { trimAndValidateIdentifier } from './identifier-validation.js';

/**
 * Identifies a downstream push-delivery consumer (story S08; Meridian
 * `IDEA-65`: "Downstream Push consumers are tracked via a dedicated
 * `delivery_subscriptions` database table"). Scoped to a workspace, not
 * globally unique.
 */
export type ConsumerId = string & { readonly __tag: 'ConsumerId' };

export function consumerId(value: string): ConsumerId {
  return trimAndValidateIdentifier('ConsumerId', value) as ConsumerId;
}
