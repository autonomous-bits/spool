# S05: Route external feedback through human-reviewed suggestions

## Business value

Stakeholders need feedback from agents and external systems to be useful without bypassing review.
External feedback should become a clear suggestion that a human can accept into branch work or
reject.

## Fidelity references

- Functional spec: `docs/specifications/feature-01-core-domain-model/functional-specification.md`
- Technical spec: `docs/specifications/feature-01-core-domain-model/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-28` / `a5e98493-1e78-47fc-b64e-c77283635f06`,
  `IDEA-40` / `dad3fe93-650c-40f7-a2c2-2d41cc837356`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can see that external feedback is pending human review before it affects approved
   context.
2. A stakeholder can accept a suggestion into branch work owned by one discipline.
3. A stakeholder can reject a suggestion without changing idea or relationship context.
4. A stakeholder can trace accepted branch work back to the suggestion that started it.
5. An agent can submit useful feedback without gaining authority to approve or merge it.

## Deliverable

TypeScript types, domain invariants, and unit tests in `apps/store/src/domain/` covering
`SuggestionState` (pending, accepted, rejected), the link between an accepted suggestion and its
discipline-scoped feedback branch, and the protected operation contracts for accepting and
rejecting suggestions. Content must align with the "Required lifecycle contracts — Suggestion" and
"Protected operation contracts — Accept suggestion, Reject suggestion" sections of the technical
specification and the Meridian neighbourhoods listed above.

## Out of scope

Suggestion storage, queue implementation, API request shape, and notification delivery are out of
scope for this story.
