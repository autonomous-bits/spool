# S10: Confirm one workspace's knowledge never appears in another

## Business value

Stakeholders across different workspaces need assurance that their approved ideas, drafts,
relationships, suggestions, feedback, and delivery preferences stay private to their own
workspace. Without this guarantee, stakeholders could not trust that sensitive or in-progress work
is contained to the workspace they intended.

## Fidelity references

- Functional spec: `docs/specifications/feature-02-postgres-persistence/functional-specification.md`
- Technical spec: `docs/specifications/feature-02-postgres-persistence/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-35` / `7db48f74-ed75-45a0-a178-5532f8396ce9`
  (feature-01 workspace scoping, reused here per the technical specification's tenant isolation
  decision)

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A workspace owner can confirm that one workspace's knowledge does not appear in another
   workspace.
2. A stakeholder can request ideas, relationships, suggestions, feedback, notifications, or
   delivery preferences and receive only records belonging to the workspace they are acting
   within.
3. An implementation agent cannot obtain records belonging to a workspace other than the one it
   is scoped to through the persistence layer.

## Deliverable

A persistence-layer guarantee in `apps/store` that every workspace-owned record carries an
unambiguous workspace association and that all read paths filter by it, with adapter-level tests
covering chunks, edges, branches, suggestions, chunk-artifact associations, delivery
subscriptions, and notifications across at least two workspaces. Content must align with the
"Tenant isolation" row of the technical specification and Functional spec acceptance criterion 6.

## Out of scope

Authentication-provider integration, API-level authorization mechanics, and workspace membership
management are out of scope for this story.
