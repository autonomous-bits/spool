# Feature 03: API Gateway — Brief

Status: pre-spec brief. Use this to generate the functional and technical specifications for this
feature (see `spool-functional-specs` and `spool-technical-specs` skills). This brief is not itself
a specification and must not be treated as authoritative once real specs exist.

## Why this feature is needed

`apps/store` currently implements the full Feature 01/02 domain and persistence layers (chunk/edge
lifecycle, branch lifecycle, merge, conflict detection, suggestions, verification, artifact
associations, delivery subscriptions), but `AppModule` only wires up a trivial `HealthController`.
`PersistenceModule` is deliberately not imported anywhere yet. There is no way for any external
client, MCP server, or downstream consumer to actually reach this logic over the network.

Meridian is explicit that a NestJS API gateway is the sole application boundary for the system
(`IDEA-34`, `IDEA-51`, `IDEA-52`), and several already-promoted chunks assume gateway routes exist
that do not yet exist in code (`IDEA-49` suggestions ingestion route, `IDEA-65` pull-query
endpoints). An implementation agent (`IDEA-70`, a promoted conflict report) already flagged that no
gateway/controller layer exists in `apps/store` beyond the health endpoint, and Meridian
subsequently amended `IDEA-35` to clarify the gateway is *not* required for domain-invariant
enforcement (the domain/persistence layers already enforce those) — but the gateway is still
required as the system's external boundary and as the transport for every other capability.

## Suggested scope for the functional spec

- Expose the existing domain/persistence capabilities (capture/approve/promote chunks, create/query
  edges, branch lifecycle transitions, suggestions ingestion and review, artifact association
  queries, downstream delivery subscription registration) to external clients through a single
  NestJS application boundary.
- Cover both Pull (direct query endpoints) and the entry points that existing delivery/notification
  infrastructure already assumes (`IDEA-65`, `IDEA-63`).
- Business acceptance criteria should focus on stakeholders and downstream systems being able to
  reach Spool's knowledge graph at all, not on route shapes or transport details.

## Suggested scope for the technical spec

- Confirm `apps/store` (not a separate service) owns this gateway, per `IDEA-34`/`IDEA-51`.
- Record that domain-invariant enforcement remains in `apps/store`'s domain/persistence layers per
  the amended `IDEA-35`; the gateway's job is translation/transport, not re-implementing invariants
  (avoid duplicating enforcement logic at the controller layer).
- Note the dependency on Feature 04 (human authentication enforcement) for the submit/verify/merge
  routes specifically — this feature should not silently take on that responsibility itself.
- Leave concrete REST routes, DTOs, and transport framework detail to the technical spec, not this
  brief.

## Meridian references to start from (snapshot 2026-07-04)

Workspace `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`.

| Meridian reference | Relevance |
| --- | --- |
| `IDEA-34` / `a83e51a9-017e-48b6-817a-cd0ed94458a5` | NestJS API gateway is the sole application boundary for external clients and MCP servers. |
| `IDEA-51` / `5141d1c3-1626-446a-b2f2-265d5ea33d18` | NestJS API Gateway container: exposes REST/gRPC endpoints, enforces domain validation, manages branch isolation and write locks. |
| `IDEA-52` / `e6e29a65-2d58-4625-8768-e036ab0c7c83` | API Gateway Controller component: handles requests, translates to domain services, manages responses. |
| `IDEA-49` / `831b6a55-23e6-4a68-ada2-5f6f8db68d30` | Suggestions ingestion route must be exposed by the gateway. |
| `IDEA-65` / `3a5102b9-3fb5-4153-adf1-7cdaed95f511` | Pull queries are served directly through gateway query endpoints; Push is a separate delivery concern. |
| `IDEA-35` (amended) / `7db48f74-ed75-45a0-a178-5532f8396ce9` | Domain invariants are enforced in `apps/store` domain/persistence layers; gateway is not the primary enforcement point. |
| `IDEA-70` / `aa168843-b780-4ecc-8c0a-84a0632da7b1` | Promoted conflict report documenting that no gateway/controller layer exists yet. |

Re-resolve these UUIDs in Meridian before relying on them if the labels ever disagree.

If Meridian content and this brief disagree, Meridian wins.
