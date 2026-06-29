# S08: Give agents traceable approved implementation context

## Business value

Implementation agents need approved context that is safe to act on, and stakeholders need confidence
that generated context can be traced back to the approved ideas and relationships that produced it.

## Fidelity references

- Functional spec: `docs/specifications/feature-01-core-domain-model/functional-specification.md`
- Technical spec: `docs/specifications/feature-01-core-domain-model/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-36` / `bd8c932c-3fd0-43fa-9405-4a386942f12b`,
  `IDEA-37` / `a5f79498-4db7-4767-af87-1efdab40921b`, `IDEA-38` /
  `ce7fc52f-1d8f-428d-b7af-8d454a059aaa`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. An implementation agent can request context that contains only approved or promoted ideas and
   currently active relationships.
2. A stakeholder can trace each generated context item back to the approved idea or relationship that
   supports it.
3. A stakeholder can confirm that generated context does not treat draft, superseded, inactive, or
   deactivated work as approved source material.
4. A stakeholder can confirm that generated documents are projections from approved knowledge, not
   the source of truth.
5. An implementation agent can use generated context while preserving the business meaning of the
   Meridian-backed ideas it came from.

## Deliverable

TypeScript types and unit tests in `apps/store/src/domain/` covering `GeneratedContextId`,
`ContextKind`, and the provenance rules that link generated context exclusively to approved or
promoted chunks and their active, label-based relationships. Content must align with the
"Generated context" section of the technical specification and the Meridian neighbourhoods listed
above.

## Out of scope

Generated document rendering, context package format, caching, delivery mechanics, API shape, and
transport details are out of scope for this story.
