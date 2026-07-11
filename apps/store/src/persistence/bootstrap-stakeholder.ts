/**
 * Fixed, documented UUID for the single bootstrap stakeholder seeded by
 * migrations/0002_seed_bootstrap_stakeholder.sql. Stakeholder registration is out of scope for
 * G01 (Atomic Chunk Capture); this constant lets app/test code reference a known-valid
 * createdByStakeholderId/updatedByStakeholderId until real stakeholder registration exists.
 */
export const BOOTSTRAP_STAKEHOLDER_ID = '00000000-0000-0000-0000-000000000001';
