/**
 * Fixed, documented identity for the single OAuth e2e fixture stakeholder seeded by
 * migrations/0006_seed_oauth_e2e_fixture_stakeholder.sql. Unlike `BOOTSTRAP_STAKEHOLDER_ID`
 * (discipline IS NULL — stakeholder registration is out of scope for G01/G02), this stakeholder
 * has a non-null `discipline` and a fixed `github_login`, so G04.SG5's Docker end-to-end exercise
 * can drive a real GitHub OAuth login/callback (against the stubbed provider) and submit a branch
 * whose discipline matches, without any interactive github.com consent screen.
 */
export const OAUTH_E2E_FIXTURE_STAKEHOLDER_ID = '00000000-0000-0000-0000-000000000002';
export const OAUTH_E2E_FIXTURE_GITHUB_LOGIN = 'spool-e2e-oauth-fixture';
export const OAUTH_E2E_FIXTURE_DISCIPLINE = 'engineering';
