# Feature 01: Core Domain Model

## Purpose

Define Spool's shared business vocabulary for turning stakeholder conversation into approved,
traceable implementation context.

This feature should stay intentionally small in this repository. The detailed source of truth for
the feature is the Meridian workspace. This file records the feature intent, stakeholder value, and
the Meridian context that must be used before implementation or technical specification work starts.

## Business value

Stakeholders and supervised agents need a common language for the knowledge they create and review:
workspaces, stakeholders, disciplines, idea chunks, relationships, branches, suggestions, feedback,
artifacts, notifications, delivery preferences, and generated context.

The value is **clarity and trust**. A stakeholder should be able to understand what an idea means,
who owns it, how it relates to other ideas, whether it is draft or approved, and whether it is safe
for an implementation agent to use.

## Authoritative Meridian context

Use Meridian workspace `dbb786ac-1d61-41c9-a46a-2c279dd50cc3` as the authoritative source for the
details of this feature.

Start from these chunks and their neighbourhoods. References are a snapshot from 2026-06-29; if a
label and UUID ever disagree, re-resolve the UUID in Meridian before using the reference.

| Meridian reference | Relevance |
| --- | --- |
| `IDEA-17` / `0a5e83d4-1838-42da-902b-5a12cd70bff8` | Branches are single-discipline units that preserve provenance and merge lineage. |
| `IDEA-28` / `a5e98493-1e78-47fc-b64e-c77283635f06` | AI/external feedback becomes human-reviewed suggestions and branches. |
| `IDEA-35` / `7db48f74-ed75-45a0-a178-5532f8396ce9` | The domain boundary must enforce branch, discipline, write-lock, and human-control invariants. |
| `IDEA-36` / `bd8c932c-3fd0-43fa-9405-4a386942f12b` | Relationships use logical idea labels. |
| `IDEA-37` / `a5f79498-4db7-4767-af87-1efdab40921b` | Label-based relationships survive branch overrides and promotions. |
| `IDEA-38` / `ce7fc52f-1d8f-428d-b7af-8d454a059aaa` | Relationship changes preserve lineage instead of destroying history. |
| `IDEA-40` / `dad3fe93-650c-40f7-a2c2-2d41cc837356` | AI agents act only as delegates under human supervision. |
| `IDEA-42` / `4f1d58f5-063b-4ef8-a69e-ce08e404fc4d` | Mainline merges are human-only. |
| `IDEA-43` / `9dec260c-e1e3-41a4-89df-3247ae681bda` | Verification feedback informs, but does not automate, human decisions. |
| `IDEA-57` / `43b26f52-bfd8-497e-aca5-8d91fc705787` | Autonomous agents are forbidden from approving or executing mainline merges. |

If Meridian content and this file disagree, Meridian wins.

## User-facing outcomes

1. Stakeholders can use a consistent vocabulary for Spool knowledge.
2. Draft knowledge is clearly separate from approved implementation context.
3. Discipline ownership is visible and respected.
4. AI-drafted or AI-suggested context remains distinguishable from human-approved context.
5. Relationships between ideas are explicit enough for humans and agents to reason about.
6. Generated context remains traceable to approved ideas and relationships.

## Acceptance criteria

1. A stakeholder can explain the key Spool concepts using Meridian-backed definitions.
2. A stakeholder can tell who owns a proposed change and which discipline should review it.
3. A stakeholder can tell whether context is draft, approved, promoted, superseded, or no longer
   active.
4. A stakeholder can distinguish advisory feedback from a human approval decision.
5. An implementation agent can receive context whose business meaning is traceable to approved
   Meridian-backed ideas.

## Out of scope

Technical requirements, database design, API design, MCP tool design, authentication mechanics,
performance targets, and storage/transport details belong in later technical specifications.
