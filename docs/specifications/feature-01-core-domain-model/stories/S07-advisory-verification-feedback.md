# S07: Treat verification feedback as advisory evidence

## Business value

Stakeholders need verification feedback from agents, tools, and reviewers to inform decisions without
letting that feedback automatically change branch state or bypass human judgment.

## Fidelity references

- Functional spec: `docs/specifications/feature-01-core-domain-model/functional-specification.md`
- Technical spec: `docs/specifications/feature-01-core-domain-model/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-35` / `7db48f74-ed75-45a0-a178-5532f8396ce9`,
  `IDEA-43` / `9dec260c-e1e3-41a4-89df-3247ae681bda`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can see verification feedback associated with the branch it evaluated.
2. A stakeholder can review passing, failing, or mixed feedback before deciding what happens next.
3. A stakeholder can confirm that feedback alone does not verify, unverify, merge, reject, reopen, or
   return a branch to draft.
4. A stakeholder can manually decide whether a branch is verified or needs more work after reviewing
   feedback.
5. An implementation agent can provide verification feedback without controlling the branch
   lifecycle.

## Deliverable

TypeScript types and unit tests in `apps/store/src/domain/` covering verification signals as
advisory-only records associated with a branch, and the domain invariant that signals cannot
automate any branch state transition. Content must align with the "Verification signals" section
of the technical specification and the Meridian neighbourhoods listed above.

## Out of scope

Feedback routing implementation, notification delivery, test runner integration, and branch state
storage are out of scope for this story.
