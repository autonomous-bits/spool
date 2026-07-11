import { describe, expect, it } from 'vitest';
import {
  DeliverySubscription,
  InvalidDeliverySubscriptionError,
  type DeliverySubscriptionProps,
} from './delivery-subscription.js';

function validProps(overrides: Partial<DeliverySubscriptionProps> = {}): DeliverySubscriptionProps {
  return {
    workspaceId: '00000000-0000-0000-0000-00000000d0fa',
    url: 'https://example.com/webhook',
    createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    ...overrides,
  };
}

describe('DeliverySubscription', () => {
  it('constructs a subscription with defaulted id, isActive, and a generated signingSecret', () => {
    const subscription = new DeliverySubscription(validProps());

    expect(subscription.id).toBeTruthy();
    expect(subscription.workspaceId).toBe('00000000-0000-0000-0000-00000000d0fa');
    expect(subscription.url).toBe('https://example.com/webhook');
    expect(subscription.disciplineFilter).toBeUndefined();
    expect(subscription.isActive).toBe(true);
    expect(subscription.signingSecret).toBeTruthy();
    expect(subscription.signingSecret.length).toBeGreaterThan(0);
  });

  it('generates a unique signingSecret per instance', () => {
    const first = new DeliverySubscription(validProps());
    const second = new DeliverySubscription(validProps());

    expect(first.signingSecret).not.toBe(second.signingSecret);
  });

  it('round-trips a persisted signingSecret verbatim when rehydrating from a stored row', () => {
    // The explicit signingSecret prop exists only for repository rehydration, never for a
    // caller-driven registration request (the API layer's DTO has no signingSecret input field).
    const subscription = new DeliverySubscription(validProps({ signingSecret: 'persisted-secret' }));

    expect(subscription.signingSecret).toBe('persisted-secret');
  });

  it.each(['http://example.com/webhook', 'ftp://example.com/webhook', 'not-a-url', ''])(
    'rejects a non-https url %j',
    (url) => {
      expect(() => new DeliverySubscription(validProps({ url }))).toThrow(
        InvalidDeliverySubscriptionError,
      );
    },
  );

  it('accepts a valid https url', () => {
    expect(() => new DeliverySubscription(validProps({ url: 'https://example.com' }))).not.toThrow();
  });

  it('accepts a valid disciplineFilter', () => {
    const subscription = new DeliverySubscription(
      validProps({ disciplineFilter: ['engineering', 'product'] }),
    );

    expect(subscription.disciplineFilter).toEqual(['engineering', 'product']);
  });

  it('treats an omitted disciplineFilter as "all disciplines"', () => {
    const subscription = new DeliverySubscription(validProps());

    expect(subscription.disciplineFilter).toBeUndefined();
  });

  it('rejects an invalid disciplineFilter value', () => {
    expect(() =>
      new DeliverySubscription(validProps({ disciplineFilter: ['not-a-real-discipline' as never] })),
    ).toThrow(InvalidDeliverySubscriptionError);
  });

  it.each(['', '   '])('rejects blank workspaceId %j', (workspaceId) => {
    expect(() => new DeliverySubscription(validProps({ workspaceId }))).toThrow(
      InvalidDeliverySubscriptionError,
    );
  });

  it.each(['', '   '])('rejects blank createdByStakeholderId %j', (createdByStakeholderId) => {
    expect(() => new DeliverySubscription(validProps({ createdByStakeholderId }))).toThrow(
      InvalidDeliverySubscriptionError,
    );
  });

  it('defaults isActive to true and allows explicit override', () => {
    const inactive = new DeliverySubscription(validProps({ isActive: false }));

    expect(inactive.isActive).toBe(false);
  });
});
