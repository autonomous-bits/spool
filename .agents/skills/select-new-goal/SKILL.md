---
name: select-new-goal
description: Compare the checked-out implementation with the Spool graph and choose the next atomic vertical goal.
---

# Select new goal

Always consult the Spool graph first. When the Spool MCP server is configured and available in your environment, use native MCP query tools (`spl_branch_list`, `spl_filter`, `spl_context`, `spl_resolve`) by default, falling back to the CLI (`spl`) if MCP is unavailable. Stakeholder context (goals, requirements, architecture decisions) lives in the Spool graph, not in repo docs — query it to find the next goal.
