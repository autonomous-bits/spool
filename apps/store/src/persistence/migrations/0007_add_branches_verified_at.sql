-- Adds the verified_at branch lifecycle timestamp ratified by Meridian IDEA-40's
-- submitted -> verified -> merged flow.
--
-- Scope is intentionally limited to the nullable timestamp column only; stakeholder attribution for
-- verification/rejection remains out of scope for this migration.
ALTER TABLE branches ADD COLUMN verified_at TIMESTAMP WITH TIME ZONE;
