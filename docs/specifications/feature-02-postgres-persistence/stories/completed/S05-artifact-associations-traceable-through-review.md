# S05: Keep supporting artifacts traceable through branch review

## Business value

Stakeholders need supporting artifacts linked to an idea — such as designs or documents used as
evidence during review — to stay correctly associated as that idea moves through branch review,
without a branch's in-progress association changes leaking into the approved mainline before
merge.

## Fidelity references

- Functional spec: `docs/specifications/feature-02-postgres-persistence/functional-specification.md`
- Technical spec: `docs/specifications/feature-02-postgres-persistence/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-62` / `57c360b1-479f-4e93-927c-a254a5efb787`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can trace which supporting artifact is associated with an idea while that
   association is still under branch review.
2. A stakeholder can confirm that changes to an artifact association made on a branch do not
   affect the mainline association until the branch is merged.
3. A stakeholder can tell the current status of an artifact association and see its prior
   associations rather than having history disappear when the association changes.

## Deliverable

A persistence adapter in `apps/store` that versions chunk-to-artifact associations per branch
(active, superseded, deactivated) using the same delta-based model as chunks and edges, with
adapter-level tests. Content must align with the "Chunk-artifact association lifecycle" row of
the technical specification and `IDEA-62`.

## Out of scope

Merge transaction mechanics, conflict detection, and delivery subscription persistence are out of
scope for this story.
