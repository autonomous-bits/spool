# S02: Keep draft branch work distinguishable from approved context

## Business value

Stakeholders need to know that work in progress on a branch never gets mistaken for approved
mainline context. Reviewers, downstream agents, and other disciplines must be able to trust that
what they see as "approved" has not been silently altered by someone else's unmerged draft.

## Fidelity references

- Functional spec: `docs/specifications/feature-02-postgres-persistence/functional-specification.md`
- Technical spec: `docs/specifications/feature-02-postgres-persistence/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-32` / `625375cd-7cce-47c6-a144-06d7b98a7bda`,
  `IDEA-33` / `b280346f-25a8-4cc2-8a31-67f6c8d7a452`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can identify current approved context in a workspace without it reflecting
   unmerged branch drafts.
2. A stakeholder can view a branch's in-progress state as the combination of that branch's own
   changes and the approved mainline, without the branch's changes affecting the mainline.
3. An implementation agent can request current approved context without treating draft or
   superseded branch work as approved.

## Deliverable

A persistence adapter and read-path implementation in `apps/store` that stores branch-specific
chunk and edge changes as branch-scoped records separate from mainline records, and computes a
branch's resolved view by combining those records with mainline at read time, together with
adapter-level tests. Content must align with the "Delta-based branch storage" row of the
technical specification and `IDEA-32`, `IDEA-33`.

## Out of scope

Divergence-marker conflict detection, atomic merge execution, suggestion persistence, and
chunk-artifact association versioning are out of scope for this story.
