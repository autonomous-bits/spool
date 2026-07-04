# Feature 04: Human Authentication Enforcement for Protected Transitions — Brief

Status: pre-spec brief. Use this to generate the functional and technical specifications for this
feature (see `spool-functional-specs` and `spool-technical-specs` skills). This brief is not itself
a specification and must not be treated as authoritative once real specs exist.

## Why this feature is needed

`apps/store`'s domain layer (`domain/branch-lifecycle.ts`, `domain/human-control.ts`) already
rejects non-human actors for submit/verify/merge based on an `ActorContext`'s `kind` field
(`isHumanActor`), and this is correct as far as it goes. But that check only validates a
caller-supplied claim — the domain has no concept of a session, credential, or transaction
signature. Nothing today proves the human/delegated distinction reaching the domain layer is true
rather than self-reported by whatever process is calling in.

Meridian requires more than a claim check for these specific transitions. `IDEA-40` requires the
gateway boundary to validate session scope and require *direct human authentication* (e.g. MFA or a
cryptographic transaction signature) for branch submit, verify, and merge — specifically to prevent
an AI agent from bypassing human control by self-reporting a delegation header. `IDEA-42` and
`IDEA-57` reinforce that merges must be human-triggered and that autonomous AI approval is
forbidden. This is a distinct feature from the general API gateway (Feature 03): it is a
security-critical authentication/authorization boundary, not general request routing, and should be
specified and reviewed as such.

## Suggested scope for the functional spec

- Business framing: stakeholders must be able to trust that only a real human — not a delegated
  agent acting on stale or spoofed credentials — ever submits, verifies, or merges a branch.
- Acceptance criteria should describe stakeholder-visible outcomes (e.g. an agent-driven request to
  merge is rejected even if it claims to act for a human) without prescribing MFA vendor or
  signature scheme.
- Out of scope: concrete authentication mechanism, token format, MFA provider, or cryptographic
  scheme — those belong in the technical spec only insofar as Meridian already commits to them, and
  otherwise remain implementation detail.

## Suggested scope for the technical spec

- Identify exactly which operations are protected: branch submit, branch verify, branch merge
  (`IDEA-40`, `IDEA-42`, `IDEA-57`). Confirm whether any other operations Meridian later adds should
  be included.
- State the anti-spoofing invariant explicitly: self-reported delegation/impersonation/actor headers
  must never be accepted as proof of direct human authentication; the gateway must independently
  verify a human-scoped credential before invoking the corresponding domain transition.
- Define the boundary between this feature and `apps/store`'s existing domain-level
  `isHumanActor`/`ActorContext` checks — this feature supplies a verified `ActorContext` to the
  domain layer; it does not replace or duplicate the domain's own invariant checks (`IDEA-35`
  amendment already assigns invariant enforcement to the domain layer).
- Depends on Feature 03 (API gateway) existing as the boundary where this verification happens.

## Meridian references to start from (snapshot 2026-07-04)

Workspace `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`.

| Meridian reference | Relevance |
| --- | --- |
| `IDEA-40` / `dad3fe93-650c-40f7-a2c2-2d41cc837356` | Gateway boundary must validate session scope and require direct human authentication (MFA/crypto signature) for submit/verify/merge. |
| `IDEA-42` / `4f1d58f5-063b-4ef8-a69e-ce08e404fc4d` | Merge operations restricted to human stakeholders; cannot be triggered or approved autonomously by AI agents. |
| `IDEA-57` / `43b26f52-bfd8-497e-aca5-8d91fc705787` | Human-Only Merging constraint: autonomous agents forbidden from executing or approving merges. |
| `IDEA-35` (amended) / `7db48f74-ed75-45a0-a178-5532f8396ce9` | Domain/persistence layers enforce invariants using the supplied `ActorContext`; this feature is responsible for making that context trustworthy. |
| `IDEA-53` / `fda8316a-06cf-4b7f-9100-65b58e8d6321` | Domain Invariant Protection Service component — the counterpart this feature's verified context feeds into. |

Re-resolve these UUIDs in Meridian before relying on them if the labels ever disagree.

If Meridian content and this brief disagree, Meridian wins.
