# S04: Track suggestions from submission through review outcome

## Business value

Stakeholders need to see the full lifecycle of a suggestion, from when it was first proposed
through whatever a human reviewer decided to do with it. Without a durable record, accepted or
rejected suggestions and their origin would be lost, making it impossible to audit what was
proposed and why it was or wasn't adopted.

## Fidelity references

- Functional spec: `docs/specifications/feature-02-postgres-persistence/functional-specification.md`
- Technical spec: `docs/specifications/feature-02-postgres-persistence/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-49` / `831b6a55-23e6-4a68-ada2-5f6f8db68d30`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can see pending, accepted, and rejected suggestions in a workspace.
2. A stakeholder can trace a branch created from an accepted suggestion back to that suggestion.
3. A stakeholder can confirm a newly submitted suggestion starts out pending human review rather
   than already accepted.
4. A stakeholder can confirm that an accept or reject decision on a suggestion is attributed to an
   authenticated human stakeholder, not to an unverified actor claim.

## Deliverable

A persistence adapter in `apps/store` that stores suggestions with their review status,
attributes accept/reject decisions to an authenticated human stakeholder rather than a
client-supplied actor claim, and maintains a durable link from any branch initiated by an accepted
suggestion back to that suggestion, with adapter-level tests. Content must align with the
"Suggestion persistence" row and the suggestion-provenance requirement in "Protected operation
contracts" of the technical specification and `IDEA-49`.

## Out of scope

Chunk-artifact association versioning, merge conflict detection, and notification routing are out
of scope for this story.
