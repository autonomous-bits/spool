# S04: Preserve human control over accountable decisions

## Business value

Stakeholders need confidence that approvals, suggestion decisions, verification decisions, and
mainline merges are accountable human decisions, not autonomous agent actions.

## Fidelity references

- Functional spec: `docs/specifications/feature-01-core-domain-model/functional-specification.md`
- Technical spec: `docs/specifications/feature-01-core-domain-model/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-28` / `a5e98493-1e78-47fc-b64e-c77283635f06`,
  `IDEA-40` / `dad3fe93-650c-40f7-a2c2-2d41cc837356`, `IDEA-42` /
  `4f1d58f5-063b-4ef8-a69e-ce08e404fc4d`, `IDEA-57` /
  `43b26f52-bfd8-497e-aca5-8d91fc705787`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can tell which human is accountable for an approval, suggestion decision, branch
   submission, verification decision, or merge decision.
2. An AI agent or external system can contribute feedback without being treated as the human decision
   maker.
3. A stakeholder can confirm that agents cannot approve chunks, accept or reject suggestions, submit
   branches, verify branches, or merge branches on their own.
4. A stakeholder can distinguish a delegated contribution from a direct human decision.

## Deliverable

TypeScript types, domain invariants, and unit tests in `apps/store/src/domain/` covering
`ActorContext` (human vs. delegated), and the protected operation contracts that require a
direct-human actor for chunk approval, suggestion acceptance/rejection, and branch
submission/verification/merge. Content must align with the "Human accountability", "Delegated
agents", and "Protected operation contracts" sections of the technical specification and the
Meridian neighbourhoods listed above.

## Out of scope

Authentication-provider integration, credential format, session implementation, and transport
headers are out of scope for this story.
