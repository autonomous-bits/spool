# S01: Use a shared workspace vocabulary

## Business value

Stakeholders need a common language for the knowledge they create and review so conversations,
approved context, and implementation work mean the same thing across disciplines and agents.

## Fidelity references

- Functional spec: `docs/specifications/feature-01-core-domain-model/functional-specification.md`
- Technical spec: `docs/specifications/feature-01-core-domain-model/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-35` / `7db48f74-ed75-45a0-a178-5532f8396ce9`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can identify the workspace where an idea, branch, relationship, suggestion,
   feedback item, artifact, notification, or generated context belongs.
2. A stakeholder can distinguish the key Spool concepts without relying on implementation-specific
   names.
3. A stakeholder can confirm that one workspace's knowledge is not treated as belonging to another
   workspace.
4. An implementation agent can trace the business meaning of a workspace concept back to the
   functional spec, technical spec, and referenced Meridian context.

## Out of scope

Database schemas, API routes, DTOs, transport details, and persistence behavior are out of scope for
this story.
