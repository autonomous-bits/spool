-- Adds authenticated reporter identity to verification_signals for the IDEA-139 auth cutover.
-- `verifier_name` deliberately stays untrusted free text (Meridian IDEA-21); the new
-- `reported_by_stakeholder_id` column separately records the verified caller's stakeholder id.
--
-- Legacy rows predate authenticated submission, so their reporter identity cannot be reconstructed
-- exactly. Backfill from the owning branch's creator as the closest persisted stakeholder-linked
-- provenance already available in-store, then make the new column required for all future writes.
ALTER TABLE verification_signals
    ADD COLUMN reported_by_stakeholder_id UUID;

UPDATE verification_signals AS vs
SET reported_by_stakeholder_id = b.created_by_stakeholder_id
FROM branches AS b
WHERE b.id = vs.branch_id
  AND vs.reported_by_stakeholder_id IS NULL;

ALTER TABLE verification_signals
    ALTER COLUMN reported_by_stakeholder_id SET NOT NULL;

ALTER TABLE verification_signals
    ADD CONSTRAINT verification_signals_reported_by_stakeholder_id_fkey
    FOREIGN KEY (reported_by_stakeholder_id) REFERENCES stakeholders(id);
