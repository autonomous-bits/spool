# Common workflows

Concrete tool sequences for the most frequent Spool MCP tasks. All examples assume you already
hold a `workspaceId`, `stakeholderId` (for delegated tools), and, where noted, a `sessionToken`
(for session tools).

## Capture a new idea chunk on a draft branch

1. `spool-search-chunks` (`q`, `discipline`) — confirm no equivalent chunk already exists.
2. `spool-create-branch` — only if you don't already have a draft `branchId` for this discipline.
3. `spool-capture-chunk` — pass the `branchId` from step 2 to scope the chunk to the branch.

## Relate two existing chunks

1. `spool-get-neighbourhood` on the `fromChunkLabel`'s chunk `id` — check for an existing edge of
   the same `type` before adding a duplicate.
2. `spool-create-edge` — `fromChunkLabel`, `toChunkLabel`, `type`, `discipline`; pass `branchId`
   to scope the edge to a draft branch.

## Propose a change without direct write access

Use `spool-submit-suggestion` instead of `spool-capture-chunk` / `spool-create-edge` when the
calling agent should not commit content directly (e.g. speculative or cross-discipline
proposals). Provide exactly one shape:

- Chunk-shaped: `label` + `content`.
- Edge-shaped: `fromChunkLabel` + `toChunkLabel` + `relationshipType`.

A human reviewer later accepts or rejects the suggestion outside this MCP server
(Meridian IDEA-75) — no `spool-*` tool performs that step.

## Attach a supporting artifact to a chunk

1. `spool-upload-artifact` — `content` (base64), `mimeType`. Keep decoded content at or below
   700,000 bytes.
2. `spool-attach-artifact-to-chunk` — pass the `artifactId` from step 1 and the target
   `chunkLabel`; add `branchId` to scope the association to a draft branch.

## Record a branch verification result

`spool-submit-verification-signal` — `branchId`, `verifierName`, `status` (`pass`/`fail`), and an
optional `reason`. This does not verify or reject the branch itself; it only records a signal
that the store's own lifecycle logic consumes.

## Error handling

Every delegated/session tool surfaces the store's own 4xx message unchanged (e.g. unknown
vocabulary value, missing workspace membership, unknown chunk/branch). Treat these as
authoritative — do not retry with guessed valid values; surface the store's message to the human
or re-query (e.g. `spool-search-chunks`) to find a valid target first.
