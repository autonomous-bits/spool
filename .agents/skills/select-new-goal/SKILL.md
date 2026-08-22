---
name: select-new-goal
description: Compare the checked-out implementation with the Spool graph and choose the next atomic vertical goal.
---

# Select new goal

Select one independently testable, user-observable vertical slice. Do not
implement it or mutate graph data while using this skill.

1. Read [evidence collection](references/evidence-collection.md), then inspect
   the code, existing goals, and selected Spool branch.
2. Choose the smallest unimplemented behavior supported by the evidence. It
   must span every layer it needs, have a focused test, and not duplicate a
   `planned`, `in_progress`, or `done` goal.
3. Report the goal, its status, related node IDs, and graph branch/snapshot.
   This skill is invoked from the built-in planning mode, so let that mode's
   normal plan/todo tracking record the goal and its sub-goals; do not create
   or maintain a separate `docs/goals/` document.

Use only these statuses for the goal and each sub-goal: `planned`,
`in_progress`, `blocked`, and `done`. New actionable work starts as `planned`;
break it into at least three ordered, testable sub-goals.
