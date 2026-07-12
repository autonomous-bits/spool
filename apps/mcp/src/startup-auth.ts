import {
  ensureAuthenticated,
  type AuthenticatedStoreSession,
  type EnsureAuthenticatedOptions,
} from './auth/login-flow.js';
import type { StoreClientConfig } from './store-client.js';

export interface StartupAuthenticationOptions {
  storeUrl: string;
  credentials: StoreClientConfig;
  ensureAuthenticatedImpl?: (
    options: EnsureAuthenticatedOptions,
  ) => Promise<AuthenticatedStoreSession>;
}

/**
 * Performs the stdio server's startup authentication preflight so first tool calls do not block on
 * interactive login. Explicit override tokens stay on the same fast path as storeFetch().
 */
export async function runStartupAuthentication(
  options: StartupAuthenticationOptions,
): Promise<void> {
  if (options.credentials.sessionTokenOverride !== undefined) {
    return;
  }

  const ensureAuthenticatedImpl = options.ensureAuthenticatedImpl ?? ensureAuthenticated;
  await ensureAuthenticatedImpl({
    storeUrl: options.storeUrl,
    workspaceId: options.credentials.workspaceId,
  });
}
