# S06: Identify conflicting changes before a branch merges

## Business value

Stakeholders need to know, before a branch is merged, whether someone else changed the same idea
or relationship on the mainline in the meantime. Without this, a merge could silently overwrite
work that both branch author and mainline reviewers believed was safe.

## Fidelity references

- Functional spec: `docs/specifications/feature-02-postgres-persistence/functional-specification.md`
- Technical spec: `docs/specifications/feature-02-postgres-persistence/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-41` / `58afb6dd-d95b-42db-8d1b-55573a0db05b`,
  `IDEA-46` / `97bf7816-cf64-4e4d-bc43-9dae857f0bb5`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can determine which mainline changes happened after a branch diverged from it.
2. A stakeholder can identify, before merging, whether the same idea, relationship, or supporting
   artifact association was changed independently on both the branch and the mainline.
3. A stakeholder who has caught up a branch with a conflicting mainline change can confirm the
   branch's point of comparison for future conflict checks has moved forward accordingly.

## Deliverable

A persistence-level conflict-detection capability in `apps/store` that records each branch's
divergence point and, given a branch, reports chunk, edge, and chunk-artifact-association changes
made independently on both branch and mainline since divergence, with adapter-level tests.
Content must align with the "Divergence tracking" and "Conflict detection scope" rows of the
technical specification and `IDEA-41`, `IDEA-46`.

## Out of scope

Atomic merge execution, post-merge history reconstruction, suggestion persistence, and delivery
subscriptions are out of scope for this story.
