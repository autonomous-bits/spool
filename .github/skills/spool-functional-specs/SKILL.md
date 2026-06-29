---
name: spool-functional-specs
description: >
  Minimal functional specification guidance for Spool. Use when creating or
  revising feature functional specs, especially docs/specifications content,
  to keep business value in-repo and defer authoritative detail to Meridian.
metadata:
  version: "1.0"
  compatibility: "Spool product/specification workflow with Meridian MCP"
---

# Spool Functional Specs

Use this skill when creating or revising Spool functional specifications.

Functional specs in this repository should be intentionally small. They should capture business
intent, stakeholder value, user-facing outcomes, acceptance criteria, and authoritative Meridian
references. They should not duplicate detailed product or architecture content from Meridian.

## Core rule

Meridian is the authoritative source of product and architecture detail.

If a functional spec and Meridian disagree, Meridian wins. The spec should make that authority
relationship explicit.

## Required structure

Use this structure by default:

1. `# Feature NN: Name`
2. `## Purpose`
3. `## Business value`
4. `## Authoritative Meridian context`
5. `## User-facing outcomes`
6. `## Acceptance criteria`
7. `## Out of scope`

## Meridian references

- Include the Meridian workspace ID.
- Reference starting chunks and their neighbourhoods.
- Include both the human label and UUID for each referenced chunk.
- Add a snapshot date and tell readers to re-resolve the UUID if labels ever disagree.
- Keep the relevance summary short and business-level; avoid restating all Meridian detail.

Example:

```markdown
Use Meridian workspace `dbb786ac-1d61-41c9-a46a-2c279dd50cc3` as the authoritative source for the
details of this feature.

Start from these chunks and their neighbourhoods. References are a snapshot from 2026-06-29; if a
label and UUID ever disagree, re-resolve the UUID in Meridian before using the reference.

| Meridian reference | Relevance |
| --- | --- |
| `IDEA-35` / `7db48f74-ed75-45a0-a178-5532f8396ce9` | Branch, discipline, write-lock, and human-control invariants. |

If Meridian content and this file disagree, Meridian wins.
```

## Business-only content

Functional specs may include:

- business purpose
- stakeholder value
- user-facing outcomes
- business acceptance criteria
- explicit out-of-scope technical areas
- Meridian references

Functional specs must not include:

- database schema, migrations, indexes, transactions, or query strategy
- API routes, DTOs, guards, request/response shapes, or transport details
- MCP tool design, protocol details, or implementation handlers
- authentication mechanism details
- performance targets or benchmark design
- storage, webhook, or worker implementation details
- code-level design, package layout, or test commands

## Fidelity checklist

Before considering a functional spec complete:

- [ ] The spec is short enough that Meridian remains the detailed source of truth.
- [ ] Business value is clear in one or two paragraphs.
- [ ] Acceptance criteria are written from a stakeholder or user point of view.
- [ ] Technical requirements are absent or explicitly out of scope.
- [ ] Meridian references include workspace ID, label, UUID, snapshot date, and neighbourhood guidance.
- [ ] The spec says Meridian wins if there is disagreement.
- [ ] A rubber-duck review has checked for business-value fidelity and technical leakage.

## Review and iteration loop

After drafting or revising a functional spec:

1. Re-read the spec against the functional-spec checklist.
2. Compare every Meridian reference in the spec with the current Meridian neighbourhoods used for the
   work.
3. Run a rubber-duck review focused on business-value fidelity, missing acceptance criteria,
   over-specificity, and technical leakage.
4. Apply every substantive correction from the review.
5. Repeat the review-and-correction loop until the reviewer finds no blocking or substantive
   non-blocking fidelity issues.
6. Only then report the spec as complete.
