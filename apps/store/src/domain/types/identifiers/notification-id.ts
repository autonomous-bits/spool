import { trimAndValidateIdentifier } from './identifier-validation.js';

export type NotificationId = string & { readonly __tag: 'NotificationId' };

export function notificationId(value: string): NotificationId {
  return trimAndValidateIdentifier('NotificationId', value) as NotificationId;
}
