/**
 * Fixed, documented UUID for the single bootstrap stakeholder seeded by
 * migrations/0002_seed_bootstrap_stakeholder.sql. Stakeholder registration is out of scope for
 * G01 (Atomic Chunk Capture); this constant lets app/test code reference a known-valid
 * createdByStakeholderId/updatedByStakeholderId until real stakeholder registration exists.
 */
export const BOOTSTRAP_STAKEHOLDER_ID = '00000000-0000-0000-0000-000000000001';

/**
 * Default name/email used to seed the bootstrap stakeholder when
 * `ADMIN_STAKEHOLDER_NAME`/`ADMIN_STAKEHOLDER_EMAIL` are not set in the environment (e.g. local
 * unit tests, or a Docker Compose run where `tools/docker/seed-admin-env.sh` hasn't been run).
 * See `migrator.ts`'s `ensureBaselineSeedData` for how these are overridden.
 */
export const DEFAULT_BOOTSTRAP_STAKEHOLDER_NAME = 'Bootstrap Stakeholder';
export const DEFAULT_BOOTSTRAP_STAKEHOLDER_EMAIL = 'bootstrap-stakeholder@spool.local';
