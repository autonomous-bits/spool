# S03: Keep branch work owned by one discipline

## Business value

Stakeholders need branch work to have clear discipline ownership so the right people can review it
and changes do not blur responsibility across product, architecture, design, or engineering
concerns.

## Fidelity references

- Functional spec: `docs/specifications/feature-01-core-domain-model/functional-specification.md`
- Technical spec: `docs/specifications/feature-01-core-domain-model/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-17` / `0a5e83d4-1838-42da-902b-5a12cd70bff8`,
  `IDEA-35` / `7db48f74-ed75-45a0-a178-5532f8396ce9`, `IDEA-40` /
  `dad3fe93-650c-40f7-a2c2-2d41cc837356`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can tell which discipline owns a branch for its lifetime.
2. A stakeholder can tell when branch work is still editable and when it has moved into review,
   verification, or merged history.
3. A stakeholder from the branch discipline can submit branch work for review.
4. A stakeholder can confirm that submitted, verified, or merged branch idea and relationship
   changes are protected from further changes.
5. A stakeholder can trace merged work back to the branch that introduced it.

## Out of scope

Conflict detection, database transactions, API authentication mechanics, and merge implementation
details are out of scope for this story.
