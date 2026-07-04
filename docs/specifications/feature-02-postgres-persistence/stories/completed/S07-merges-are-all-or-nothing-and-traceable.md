# S07: Trust that a branch merge either fully lands or leaves nothing changed

## Business value

Stakeholders need certainty that merging a branch never leaves the mainline in a half-changed
state, and that after a merge they can still explain how the merged content came to be. Partial
merges or lost provenance would undermine trust in what "approved" means.

## Fidelity references

- Functional spec: `docs/specifications/feature-02-postgres-persistence/functional-specification.md`
- Technical spec: `docs/specifications/feature-02-postgres-persistence/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-47` / `7d87db29-8098-45a9-b0dc-8c8879ca66dc`,
  `IDEA-69` / `668097d0-1e19-43d7-bfbe-f81a67669827`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can confirm that a failed merge attempt leaves no partial changes to ideas,
   relationships, supporting artifact associations, or branch status.
2. A stakeholder can confirm that a successful merge applies all of its changes together, with
   none observable in isolation before the others.
3. A stakeholder can trace merged ideas, relationships, and supporting artifact associations back
   to the branch and review process that produced them, even after the merge has completed.

## Deliverable

A merge-execution capability in `apps/store` that applies all persisted graph mutations of a
merge as a single all-or-nothing operation and preserves enough origin information that a merged
branch's pre-merge state remains reconstructable afterward, with adapter-level tests including a
forced-failure rollback case. Content must align with the "Atomic merge" and "Pre-merge history
reconstruction" rows of the technical specification and `IDEA-47`, `IDEA-69`.

## Out of scope

Conflict detection prior to merge, suggestion-to-branch linkage, notification routing, and
delivery subscriptions are out of scope for this story.
