# S09: Remember feedback and verification notifications without losing the record

## Business value

Stakeholders need to be alerted when evaluation feedback or verification signals come in on their
branch, and need to be able to acknowledge those alerts without ever losing the underlying
feedback record — the history of what was said must remain available for review regardless of
whether it has been read.

## Fidelity references

- Functional spec: `docs/specifications/feature-02-postgres-persistence/functional-specification.md`
- Technical spec: `docs/specifications/feature-02-postgres-persistence/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-67` / `694ccd73-cf6d-4411-b83a-5d68296892c6`,
  `IDEA-68` / `8a9a9d0f-3a7d-425e-81c9-5e70ddf097af`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can see verification feedback attached to the branch it evaluated.
2. The author of an evaluated branch, at minimum, receives a notification as soon as evaluation
   feedback is submitted, whether or not they are online at that moment.
3. A stakeholder can acknowledge a review notification without losing the historical feedback or
   verification signal record it references.
4. A stakeholder can confirm that recording feedback or a verification signal does not, by
   itself, change a branch's lifecycle state.
5. A stakeholder can confirm that a verification signal or feedback record is attributed to an
   authenticated human stakeholder where a human reviewer is the source, not to an unverified
   actor claim.

## Deliverable

A persistence adapter in `apps/store` that routes and stores evaluation feedback and verification
signals as notification records immediately on ingestion, linked to the branch they evaluated,
attributes human-sourced signals to an authenticated stakeholder rather than a client-supplied
actor claim, and supports acknowledgement without mutating or deleting the underlying record, with
adapter-level tests. Content must align with the "Feedback notification routing" and
"Notification acknowledgement is non-destructive" rows and the provenance requirement in
"Protected operation contracts" of the technical specification, and `IDEA-67`, `IDEA-68`.

## Out of scope

Delivery subscription persistence, suggestion persistence, and merge transaction mechanics are out
of scope for this story.
