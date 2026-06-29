# S02: See whether idea context is safe to use

## Business value

Stakeholders and agents need to know whether an idea is still draft work, approved for use, promoted
to mainline context, superseded by later work, or no longer active.

## Fidelity references

- Functional spec: `docs/specifications/feature-01-core-domain-model/functional-specification.md`
- Technical spec: `docs/specifications/feature-01-core-domain-model/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-35` / `7db48f74-ed75-45a0-a178-5532f8396ce9`,
  `IDEA-38` / `ce7fc52f-1d8f-428d-b7af-8d454a059aaa`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can tell whether an idea chunk is draft, approved, promoted, superseded, or
   inactive.
2. A stakeholder can tell that draft context is not approved implementation context.
3. An implementation agent can receive only context that is safe for implementation use when it asks
   for approved context.
4. A stakeholder can tell when an approved or promoted idea has been replaced without losing the
   fact that it previously existed.

## Out of scope

Storage strategy, query strategy, API shape, and generated document rendering are out of scope for
this story.
