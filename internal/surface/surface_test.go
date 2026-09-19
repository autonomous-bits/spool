package surface

import (
	"testing"
)

func TestKeepAndRemovedAreDisjoint(t *testing.T) {
	assertDisjoint(t, "CLI", KeepCLITopLevel, RemovedCLITopLevel)
	assertDisjoint(t, "MCP", KeepMCPTools, RemovedMCPTools)
}

func TestQueryContextRenameHasNoOldNameOnKeepSurface(t *testing.T) {
	if !contains(KeepCLITopLevel, "query-context") {
		t.Fatal("KEEP CLI must include query-context")
	}
	if !contains(KeepCLITopLevel, "context") {
		t.Fatal("KEEP CLI must include context (init/export parent)")
	}
	if !contains(KeepMCPTools, "spl_query_context") {
		t.Fatal("KEEP MCP must include spl_query_context")
	}
	if contains(KeepMCPTools, "spl_context") {
		t.Fatal("spl_context is the old query tool id and must not be KEEP")
	}
	if !contains(RemovedMCPTools, "spl_context") {
		t.Fatal("spl_context must stay on the REMOVE golden list")
	}
	for _, name := range []string{"add", "status", "commit", "branch", "switch"} {
		if contains(KeepCLITopLevel, name) {
			t.Errorf("Spool VCS wrapper %q must not be KEEP", name)
		}
		if !contains(RemovedCLITopLevel, name) {
			t.Errorf("Spool VCS wrapper %q must stay on the REMOVE golden list", name)
		}
	}
}

func TestMutateIsKeepAndVCSWrappersStayRemoved(t *testing.T) {
	if !contains(KeepCLITopLevel, "mutate") {
		t.Fatal("KEEP CLI must include mutate")
	}
	if !contains(KeepMCPTools, "spl_mutate") {
		t.Fatal("KEEP MCP must include spl_mutate")
	}
	for _, name := range []string{"spl_add", "spl_commit", "spl_status", "spl_branch_list", "spl_branch_create", "spl_branch_delete", "spl_switch"} {
		if contains(KeepMCPTools, name) {
			t.Errorf("removed MCP twin %q must not be KEEP", name)
		}
		if !contains(RemovedMCPTools, name) {
			t.Errorf("removed MCP twin %q must stay on the REMOVE golden list", name)
		}
	}
}

func assertDisjoint(t *testing.T, kind string, keep, removed []string) {
	t.Helper()
	seen := map[string]struct{}{}
	for _, name := range keep {
		if name == "" {
			t.Errorf("%s KEEP contains an empty name", kind)
		}
		if _, ok := seen[name]; ok {
			t.Errorf("%s KEEP duplicates %q", kind, name)
		}
		seen[name] = struct{}{}
	}
	for _, name := range removed {
		if name == "" {
			t.Errorf("%s REMOVE contains an empty name", kind)
		}
		if _, ok := seen[name]; ok {
			t.Errorf("%s name %q is on both KEEP and REMOVE", kind, name)
		}
	}
}

func contains(list []string, want string) bool {
	for _, have := range list {
		if have == want {
			return true
		}
	}
	return false
}
