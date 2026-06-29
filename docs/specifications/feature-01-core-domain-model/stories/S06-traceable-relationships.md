# S06: Keep relationships traceable as ideas evolve

## Business value

Stakeholders and agents need relationships between ideas to remain meaningful as ideas are branched,
overridden, promoted, superseded, or deactivated.

## Fidelity references

- Functional spec: `docs/specifications/feature-01-core-domain-model/functional-specification.md`
- Technical spec: `docs/specifications/feature-01-core-domain-model/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-36` / `bd8c932c-3fd0-43fa-9405-4a386942f12b`,
  `IDEA-37` / `a5f79498-4db7-4767-af87-1efdab40921b`, `IDEA-38` /
  `ce7fc52f-1d8f-428d-b7af-8d454a059aaa`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can see relationships between ideas using stable idea labels.
2. A stakeholder can tell which relationship is currently active when earlier relationships have
   been replaced.
3. A stakeholder can trace a replaced relationship back through its prior versions.
4. An implementation agent receives a relationship view that does not contain conflicting active
   meanings for the same source idea, target idea, and relationship type.
5. A stakeholder can confirm that replacing or deactivating a mainline relationship does not erase
   its history.

## Deliverable

TypeScript types, domain invariants, and unit tests in `apps/store/src/domain/` covering
`IdeaLabel`, `RelationshipType`, `EdgeState` (active, deactivated, superseded), edge determinism
(at most one active edge per source label–target label–type triple), and the lineage-preservation
rule that mainline relationship changes must supersede prior versions rather than delete them.
Content must align with the "Logical edge endpoints", "Edge determinism", "Edge lineage", and
"Required lifecycle contracts — Edge" sections of the technical specification and the Meridian
neighbourhoods listed above.

## Out of scope

Database foreign keys, relationship table design, indexes, merge transactions, and query algorithms
are out of scope for this story.
