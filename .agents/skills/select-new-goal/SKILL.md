---
name: select-new-goal
description: Compare the checked-out implementation with the Spool graph, choose the next atomic vertical goal, and save it under docs/goals/.
---

# Select new goal

Select one independently testable, user-observable vertical slice. Do not
implement it or mutate graph data while using this skill.

1. Read [evidence collection](references/evidence-collection.md), then inspect
   the code, existing goals, and selected Spool branch.
2. Choose the smallest unimplemented behavior supported by the evidence. It
   must span every layer it needs, have a focused test, and not duplicate a
   `planned`, `in_progress`, or `done` goal.
3. Create `docs/goals/<NNN>-<short-kebab-case-slug>.html` using
   [the goal template](references/goal-template.html). `NNN` is the next unused
   three-digit number.
4. Report the document path, goal status, related node IDs, and graph
   branch/snapshot.

Use only these statuses for the goal and each sub-goal: `planned`,
`in_progress`, `blocked`, and `done`. New actionable work starts as `planned`;
create at least three ordered, testable sub-goals. `docs/goals/` is a local,
Git-ignored planning artifact, so create it if necessary and update the same
file when statuses change.
