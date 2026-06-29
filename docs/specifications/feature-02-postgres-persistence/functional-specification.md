# Feature 02: Persistent Knowledge Store

## Purpose

Ensure Spool reliably remembers workspace knowledge, review history, and approved context over time.

This feature should stay intentionally small in this repository. The detailed source of truth for
the feature is the Meridian workspace. This file records the business intent, stakeholder value, and
the Meridian context that must be used before implementation or technical specification work starts.

## Business value

Stakeholders and agents need continuity. Approved ideas, draft work, relationships, branch history,
suggestions, feedback, notifications, delivery preferences, and provenance must survive beyond a
single chat session.

The value is **durability and auditability**. A stakeholder should be able to return later and
understand what is currently approved, what is under review, what changed, who was accountable, and
why the current context is safe to use.

## Authoritative Meridian context

Use Meridian workspace `dbb786ac-1d61-41c9-a46a-2c279dd50cc3` as the authoritative source for the
details of this feature.

Start from these chunks and their neighbourhoods. References are a snapshot from 2026-06-29; if a
label and UUID ever disagree, re-resolve the UUID in Meridian before using the reference.

| Meridian reference | Relevance |
| --- | --- |
| `IDEA-31` / `1b0bd656-e365-4c79-a85e-e52066895dc5` | The versioned knowledge graph must be durably stored. |
| `IDEA-32` / `625375cd-7cce-47c6-a144-06d7b98a7bda` | Branch changes are preserved as deltas rather than full copies. |
| `IDEA-33` / `b280346f-25a8-4cc2-8a31-67f6c8d7a452` | Branch-visible knowledge combines proposed changes with approved context. |
| `IDEA-38` / `ce7fc52f-1d8f-428d-b7af-8d454a059aaa` | Relationship history is preserved through superseding changes. |
| `IDEA-41` / `58afb6dd-d95b-42db-8d1b-55573a0db05b` | Branch divergence supports later conflict understanding. |
| `IDEA-46` / `97bf7816-cf64-4e4d-bc43-9dae857f0bb5` | Conflicting changes must be identifiable before merge. |
| `IDEA-47` / `7d87db29-8098-45a9-b0dc-8c8879ca66dc` | Merge outcomes must be all-or-nothing from the user's point of view. |
| `IDEA-49` / `831b6a55-23e6-4a68-ada2-5f6f8db68d30` | Suggestions and their review outcomes must be remembered. |
| `IDEA-62` / `57c360b1-479f-4e93-927c-a254a5efb787` | Supporting artifact associations must remain traceable through branch review. |
| `IDEA-63` / `9d230f56-161d-42c2-85ad-5ef5093e0edf` | Approved context changes must support downstream delivery. |
| `IDEA-65` / `3a5102b9-3fb5-4153-adf1-7cdaed95f511` | Delivery preferences must be remembered for downstream consumers. |
| `IDEA-67` / `694ccd73-cf6d-4411-b83a-5d68296892c6` | Feedback notifications must be remembered for stakeholder review. |
| `IDEA-68` / `8a9a9d0f-3a7d-425e-81c9-5e70ddf097af` | Verification feedback must be routed and retained for human review. |
| `IDEA-69` / `668097d0-1e19-43d7-bfbe-f81a67669827` | Merged branch contributions must remain reconstructable for history and provenance. |

If Meridian content and this file disagree, Meridian wins.

## User-facing outcomes

1. Stakeholders can return to a workspace and find previously captured knowledge.
2. Approved context remains separate from draft or in-review work.
3. Historical context remains available to explain why current context changed.
4. Suggestions, feedback, and notification history remain visible to accountable stakeholders.
5. Merged work remains traceable to the branch or review process that produced it.
6. Downstream consumers can rely on remembered delivery preferences for approved context updates.

## Acceptance criteria

1. A stakeholder can identify current approved context in a workspace.
2. A stakeholder can trace an approved idea or relationship back to its review history.
3. A stakeholder can see pending, accepted, and rejected suggestions.
4. A stakeholder can see verification feedback attached to the branch it evaluated.
5. A stakeholder can acknowledge review notifications without losing the historical feedback record.
6. A workspace owner can confirm that one workspace's knowledge does not appear in another
   workspace.
7. An implementation agent can request current approved context without treating draft or
   superseded work as approved.

## Out of scope

Technical requirements, database table design, migrations, indexes, transactions, query strategy,
API design, MCP tool design, authentication mechanics, performance targets, and storage/transport
details belong in later technical specifications.
