# S08: Remember downstream delivery preferences for approved updates

## Business value

Downstream consumers of Spool's approved context need their delivery preferences to persist so
they keep receiving relevant updates without re-registering every session, while stakeholders
need merges to complete without waiting on delivery to those consumers.

## Fidelity references

- Functional spec: `docs/specifications/feature-02-postgres-persistence/functional-specification.md`
- Technical spec: `docs/specifications/feature-02-postgres-persistence/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-63` / `9d230f56-161d-42c2-85ad-5ef5093e0edf`,
  `IDEA-65` / `3a5102b9-3fb5-4153-adf1-7cdaed95f511`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A downstream consumer's delivery preferences, including any discipline filter, remain
   registered across sessions without needing to be re-submitted.
2. A stakeholder can confirm that a branch merge completes without waiting for downstream push
   delivery to finish.
3. A stakeholder can rely on on-demand access to approved context being available independently
   of whether push delivery to any given consumer has succeeded.

## Deliverable

A persistence adapter in `apps/store` that stores durable, workspace-scoped delivery subscription
records (including discipline filters) independent of individual delivery attempts, and ensures
push delivery triggered by merge runs asynchronously rather than inside the merge transaction,
with adapter-level tests. Content must align with the "Downstream delivery split" and "Delivery
subscription persistence" rows of the technical specification and `IDEA-63`, `IDEA-65`.

## Out of scope

Merge transaction mechanics, suggestion persistence, and notification routing for feedback are out
of scope for this story.
