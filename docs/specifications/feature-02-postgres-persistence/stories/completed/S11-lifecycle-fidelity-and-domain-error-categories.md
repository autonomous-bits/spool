# S11: Trust that persistence never blurs a lifecycle state or hides a failure's true cause

## Business value

Stakeholders need every lifecycle state an idea, relationship, branch, or suggestion can be in to
remain distinguishable once stored, and they need failures to be reported in terms they already
understand from how Spool's domain works, not as unexplained persistence errors. Losing that
distinction or clarity would make it impossible to trust what "approved," "superseded," or
"rejected" currently means, or to understand why an operation was refused.

## Fidelity references

- Functional spec: `docs/specifications/feature-02-postgres-persistence/functional-specification.md`
- Technical spec: `docs/specifications/feature-02-postgres-persistence/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-38` / `ce7fc52f-1d8f-428d-b7af-8d454a059aaa`,
  `IDEA-46` / `97bf7816-cf64-4e4d-bc43-9dae857f0bb5`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can distinguish every lifecycle state a chunk, branch, suggestion, or
   relationship can be in, exactly as defined for the domain model, with no two distinct states
   collapsing into the same stored representation.
2. A stakeholder can distinguish a relationship that has been superseded by a newer version from
   one that has simply been retired with no replacement.
3. A stakeholder who encounters a rejected operation receives a reason that matches one of the
   domain's established categories of failure (for example: not found, invalid state change,
   unauthorized actor, write locked, discipline boundary violation, branch isolation violation,
   duplicate active relationship, lineage violation, or tenant boundary violation) rather than an
   unexplained or ad hoc persistence failure.
4. A stakeholder can confirm that a conflict discovered during merge is reported as one of those
   same established failure categories rather than as a new kind of error.

## Deliverable

A persistence-adapter-level guarantee in `apps/store` that every lifecycle state and transition
defined for chunks, branches, suggestions, and edges is representable without lossy collapsing,
and that persistence failures are surfaced using the domain's existing error categories, with
adapter-level tests covering each lifecycle state pair that must stay distinguishable (including
superseded vs. deactivated edges) and each required error category. Content must align with the
"Required lifecycle contracts" and "Required domain error categories" sections of the technical
specification, `IDEA-38`, and `IDEA-46`.

## Out of scope

Defining new lifecycle states or error categories, API-level error response formatting, and
authentication mechanics are out of scope for this story.
