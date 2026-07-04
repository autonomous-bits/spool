# S03: Keep relationship history intact as relationships change

## Business value

Stakeholders need confidence that when a relationship between ideas changes or is retired, the
prior meaning is not lost. Understanding why current context is safe to use depends on being able
to see what a relationship used to be before it was replaced.

## Fidelity references

- Functional spec: `docs/specifications/feature-02-postgres-persistence/functional-specification.md`
- Technical spec: `docs/specifications/feature-02-postgres-persistence/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-38` / `ce7fc52f-1d8f-428d-b7af-8d454a059aaa`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can trace an approved relationship back to the version it replaced, however many
   times it has changed.
2. A stakeholder can always find the history of a relationship, even after it has been replaced
   or retired, rather than the history simply disappearing.
3. A stakeholder can confirm that changing what kind of relationship connects two ideas still
   leaves a traceable path from the earlier relationship to the current one.
4. An implementation agent always receives an unambiguous, single current relationship between any
   two ideas for a given relationship meaning.

## Deliverable

A persistence adapter in `apps/store` that stores relationship (edge) records so that
supersession creates a new lineage-linked version rather than deleting or overwriting the prior
one, with adapter-level tests proving lineage is preserved across repeated supersession and type
changes. Content must align with the "Edge lineage persistence" and "Logical edge identity in
persistence" rows of the technical specification and `IDEA-38`.

## Out of scope

Delta-based branch storage, merge conflict detection, and merge transaction mechanics are out of
scope for this story.
