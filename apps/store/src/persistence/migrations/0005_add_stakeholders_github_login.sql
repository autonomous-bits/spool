-- Adds the github_login column ratified by Meridian IDEA-81 (ADR, promoted): GitHub OAuth
-- authorization-code flow is the concrete human-authentication mechanism for branch
-- submit/verify/merge. The store maps a resolved GitHub identity (github.com's /user endpoint
-- `login` field) to an existing stakeholder record via this column.
--
-- Nullable + unique: existing stakeholders (e.g. the bootstrap stakeholder seeded by
-- 0002_seed_bootstrap_stakeholder.sql) have no GitHub identity yet, and Postgres unique
-- constraints treat multiple NULLs as distinct, so this does not require backfilling every row.
ALTER TABLE stakeholders ADD COLUMN github_login VARCHAR(255) UNIQUE;
