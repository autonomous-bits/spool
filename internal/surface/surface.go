// Package surface is the golden KEEP/REMOVE command and MCP tool list after the
// git-SoT cut (#110). Tests fail if a removed name reappears in help or tools/list.
package surface

// KeepCLITopLevel is the exact set of top-level Cobra commands advertised by spl --help.
var KeepCLITopLevel = []string{
	"asset", "completion", "context", "filter", "graph", "help", "mcp",
	"merge", "mutate", "prune", "query-context", "resolve", "schema", "search",
	"search-expand", "validate", "version",
}

// RemovedCLITopLevel is deleted from CLI. Golden tests fail if any name is registered
// or listed as an available command. There are no aliases or deprecation stubs.
var RemovedCLITopLevel = []string{
	"init", "workspace", "remote", "push", "pull", "clone", "migrate",
	"fsck", "gc", "cherry-pick", "add", "status", "commit", "branch",
	"switch", "history", "diff", "branches-containing",
}

// KeepMCPTools is the exact MCP tools/list after the git-SoT cut.
var KeepMCPTools = []string{
	"spl_asset_add",
	"spl_asset_read",
	"spl_context_export",
	"spl_filter",
	"spl_graph",
	"spl_merge_abort",
	"spl_merge_apply",
	"spl_merge_conflicts",
	"spl_merge_finalize",
	"spl_merge_preview",
	"spl_merge_resolve",
	"spl_mutate",
	"spl_prune",
	"spl_query_context",
	"spl_resolve",
	"spl_schema_migrate",
	"spl_search",
	"spl_search_expand",
	"spl_validate",
	"spl_version",
}

// RemovedMCPTools is deleted from MCP. Golden tests fail if any name is registered.
// Includes the pre-cut Rack/VCS twins so they cannot sneak back even if added to KEEP.
var RemovedMCPTools = []string{
	"spl_add",
	"spl_status",
	"spl_commit",
	"spl_init",
	"spl_context",
	"spl_diff",
	"spl_history",
	"spl_branches_containing",
	"spl_branch_list",
	"spl_branch_create",
	"spl_branch_delete",
	"spl_switch",
	"spl_cherry_pick",
	"spl_fsck",
	"spl_gc",
	"spl_migrate",
	"spl_workspace_init",
	"spl_workspace_attach",
	"spl_remote_set",
	"spl_remote_show",
	"spl_remote_remove",
	"spl_remote_branch",
	"spl_push",
	"spl_pull",
	"spl_clone",
}
