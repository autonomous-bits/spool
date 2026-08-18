package repository

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository/branch"
)

func TestCanonicalObjectIdentityUsesContentTypeAndEncoding(t *testing.T) {
	first := map[string]string{}
	first["alpha"] = "one"
	first["beta"] = "two"
	second := map[string]string{}
	second["beta"] = "two"
	second["alpha"] = "one"

	const canonicalFixtureID ObjectID = "84d795f2ecfdd03ad938173fd4be707b7f1e99f91dc1c8ac0d9077364ae3a752"
	got := persistedObjectID("fixture", first)
	if got != canonicalFixtureID {
		t.Fatalf("canonical fixture ID = %q, want %q", got, canonicalFixtureID)
	}

	if got != persistedObjectID("fixture", second) {
		t.Fatalf("different map insertion order changed ID: %q != %q", got, persistedObjectID("fixture", second))
	}
	if got == persistedObjectID("other-fixture", first) {
		t.Fatal("same content with a distinct type tag produced the same ID")
	}

	repo := NewSeedRepository()
	if stored := repo.store("fixture", second); stored != got {
		t.Fatalf("stored ID = %q, want canonical ID %q", stored, got)
	}
}

func BenchmarkResolvePinned(b *testing.B) {
	repo := NewSeedRepository()
	commit, err := repo.PinBranch("main")
	if err != nil {
		b.Fatalf("PinBranch: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := repo.ResolvePinned(commit, SeedNodeID); err != nil {
			b.Fatalf("ResolvePinned: %v", err)
		}
	}
}

func TestObjectIDsPreserveImmutableContentVersions(t *testing.T) {
	repo := NewSeedRepository()
	original := Node{ID: SeedNodeID, Title: "first title"}
	updated := Node{ID: SeedNodeID, Title: "updated title"}

	originalID := repo.store("node", original)
	updatedID := repo.store("node", updated)
	if originalID == updatedID {
		t.Fatal("changed content produced the original object ID")
	}

	originalBytes, err := canonicalCBOR.Marshal(original)
	if err != nil {
		t.Fatalf("marshal original: %v", err)
	}
	updatedBytes, err := canonicalCBOR.Marshal(updated)
	if err != nil {
		t.Fatalf("marshal updated: %v", err)
	}
	if !bytes.Equal(repo.objects[originalID], originalBytes) {
		t.Fatal("original object bytes changed after storing updated content")
	}
	if !bytes.Equal(repo.objects[updatedID], updatedBytes) {
		t.Fatal("updated object bytes were not stored under its object ID")
	}
}

func TestResolvePinnedDoesNotRepairMissingProjection(t *testing.T) {
	repo := NewSeedRepository()
	commit, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	snapshot := repo.snapshots[repo.commits[commit].Snapshot]
	delete(repo.projections, snapshot.NodeRoot)

	if _, err := repo.ResolvePinned(commit, SeedNodeID); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("ResolvePinned error = %v, want ErrNodeNotFound", err)
	}
	if _, exists := repo.projections[snapshot.NodeRoot]; exists {
		t.Fatal("ResolvePinned rebuilt or repaired a missing projection")
	}
}

func TestEntityIDRemainsStableAcrossCommits(t *testing.T) {
	repo := NewSeedRepository()
	const entityID = SeedNodeID
	original := Node{ID: entityID, Title: "original title"}
	updated := Node{ID: entityID, Title: "updated title"}

	originalNodeID := repo.store("node", original)
	updatedNodeID := repo.store("node", updated)
	if originalNodeID == updatedNodeID {
		t.Fatal("updated entity content produced the original object ID")
	}

	originalNodeRoot := repo.store("prolly-node-root", []ObjectID{originalNodeID})
	updatedNodeRoot := repo.store("prolly-node-root", []ObjectID{updatedNodeID})
	edgeRoot := repo.store("prolly-edge-root", []ObjectID{})
	outAdjRoot := repo.store("prolly-out-adjacency-root", []ObjectID{})
	inAdjRoot := repo.store("prolly-in-adjacency-root", []ObjectID{})
	schemaRoot := repo.store("schema-root", map[string]string{"version": "v1"})
	originalSnapshot := graphSnapshot{
		NodeRoot: originalNodeRoot, EdgeRoot: edgeRoot, OutAdjRoot: outAdjRoot,
		InAdjRoot: inAdjRoot, SchemaRoot: schemaRoot,
	}
	updatedSnapshot := graphSnapshot{
		NodeRoot: updatedNodeRoot, EdgeRoot: edgeRoot, OutAdjRoot: outAdjRoot,
		InAdjRoot: inAdjRoot, SchemaRoot: schemaRoot,
	}
	originalSnapshotID := repo.store("graph-snapshot", originalSnapshot)
	updatedSnapshotID := repo.store("graph-snapshot", updatedSnapshot)
	repo.snapshots[originalSnapshotID] = originalSnapshot
	repo.snapshots[updatedSnapshotID] = updatedSnapshot
	repo.projections[originalNodeRoot] = map[string]Node{entityID: original}
	repo.projections[updatedNodeRoot] = map[string]Node{entityID: updated}

	originalCommitID := repo.store("commit", commit{Snapshot: originalSnapshotID, Message: "original version"})
	updatedCommitID := repo.store("commit", commit{Snapshot: updatedSnapshotID, Message: "updated version"})
	repo.commits[originalCommitID] = commit{Snapshot: originalSnapshotID, Message: "original version"}
	repo.commits[updatedCommitID] = commit{Snapshot: updatedSnapshotID, Message: "updated version"}
	repo.branches["original"] = originalCommitID
	repo.branches["updated"] = updatedCommitID

	originalResult, err := repo.ResolvePinned(originalCommitID, entityID)
	if err != nil {
		t.Fatalf("resolve original version: %v", err)
	}
	updatedResult, err := repo.ResolvePinned(updatedCommitID, entityID)
	if err != nil {
		t.Fatalf("resolve updated version: %v", err)
	}
	if originalResult.Node != original {
		t.Fatalf("original node = %#v, want %#v", originalResult.Node, original)
	}
	if updatedResult.Node != updated {
		t.Fatalf("updated node = %#v, want %#v", updatedResult.Node, updated)
	}
	if originalResult.Commit == updatedResult.Commit {
		t.Fatal("different entity versions resolved from the same commit")
	}

	originalBytes, err := canonicalCBOR.Marshal(original)
	if err != nil {
		t.Fatalf("marshal original: %v", err)
	}
	updatedBytes, err := canonicalCBOR.Marshal(updated)
	if err != nil {
		t.Fatalf("marshal updated: %v", err)
	}
	if !bytes.Equal(repo.objects[originalNodeID], originalBytes) {
		t.Fatal("original object bytes changed after storing updated entity content")
	}
	if !bytes.Equal(repo.objects[updatedNodeID], updatedBytes) {
		t.Fatal("updated object bytes were not stored under its object ID")
	}
}

func TestCreateBranchUsesExplicitBranchOrCommitSource(t *testing.T) {
	repo := NewSeedRepository()
	mainHead := repo.branches["main"]

	branchHead, err := repo.CreateBranch("from-main", branch.Source{Branch: "main"})
	if err != nil {
		t.Fatalf("CreateBranch from branch: %v", err)
	}
	if branchHead.Commit != string(mainHead) || repo.branches["from-main"] != mainHead {
		t.Fatalf("branch source head = %q, ref = %q, want %q", branchHead.Commit, repo.branches["from-main"], mainHead)
	}

	advanced, err := repo.AdvanceBranch("main")
	if err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	commitHead, err := repo.CreateBranch("from-commit", branch.Source{Commit: string(mainHead)})
	if err != nil {
		t.Fatalf("CreateBranch from commit: %v", err)
	}
	if commitHead.Commit != string(mainHead) || repo.branches["from-commit"] != mainHead {
		t.Fatalf("commit source head = %q, ref = %q, want %q", commitHead.Commit, repo.branches["from-commit"], mainHead)
	}
	if repo.branches["main"] != advanced {
		t.Fatalf("main head = %q, want %q", repo.branches["main"], advanced)
	}
}

func TestCreateBranchRejectsInvalidOrUnresolvedSource(t *testing.T) {
	repo := NewSeedRepository()

	if _, err := repo.CreateBranch("empty", branch.Source{}); !errors.Is(err, branch.ErrMissingSource) {
		t.Fatalf("empty source error = %v, want ErrMissingSource", err)
	}
	if _, err := repo.CreateBranch("ambiguous", branch.Source{Branch: "main", Commit: string(repo.branches["main"])}); !errors.Is(err, branch.ErrAmbiguousSource) {
		t.Fatalf("ambiguous source error = %v, want ErrAmbiguousSource", err)
	}
	if _, err := repo.CreateBranch("missing-branch", branch.Source{Branch: "missing"}); !errors.Is(err, branch.ErrSourceNotFound) {
		t.Fatalf("missing branch source error = %v, want ErrSourceNotFound", err)
	}
	if _, err := repo.CreateBranch("missing-commit", branch.Source{Commit: "missing"}); !errors.Is(err, branch.ErrSourceNotFound) {
		t.Fatalf("missing commit source error = %v, want ErrSourceNotFound", err)
	}
}

func TestCreateBranchRejectsMissingSourceBeforeDuplicateName(t *testing.T) {
	repo := NewSeedRepository()
	originalMain := repo.branches["main"]

	if _, err := repo.CreateBranch("main", branch.Source{Branch: "missing"}); !errors.Is(err, branch.ErrSourceNotFound) {
		t.Fatalf("missing source error = %v, want ErrSourceNotFound", err)
	}
	if got := repo.branches["main"]; got != originalMain {
		t.Fatalf("main head = %q, want unchanged %q", got, originalMain)
	}
}

func TestCreateBranchRejectsDuplicateNameAfterSourceResolves(t *testing.T) {
	repo := NewSeedRepository()
	originalMain := repo.branches["main"]

	if _, err := repo.CreateBranch("main", branch.Source{Branch: "main"}); !errors.Is(err, branch.ErrAlreadyExists) {
		t.Fatalf("duplicate branch error = %v, want ErrAlreadyExists", err)
	}

	if got := repo.branches["main"]; got != originalMain {
		t.Fatalf("main head = %q, want unchanged %q", got, originalMain)
	}
}

func TestListBranchesReturnsSortedLocalBranchNames(t *testing.T) {
	repo := NewSeedRepository()
	if _, err := repo.CreateBranch("zebra", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch zebra: %v", err)
	}

	if _, err := repo.CreateBranch("alpha", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch alpha: %v", err)
	}

	result, err := repo.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	want := []string{"alpha", "main", "zebra"}
	if !reflect.DeepEqual(result.Branches, want) {
		t.Fatalf("branches = %#v, want %#v", result.Branches, want)
	}
}

func TestDeleteBranchDeletesExistingNonDefaultBranch(t *testing.T) {
	repo := NewSeedRepository()
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: defaultBranchName}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	result, err := repo.DeleteBranch("feature")
	if err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if result != (branch.DeleteResult{Name: "feature"}) {
		t.Fatalf("result = %#v", result)
	}
	if _, exists := repo.branches["feature"]; exists {
		t.Fatal("deleted branch remains in refs")
	}
	resultList, err := repo.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if !reflect.DeepEqual(resultList.Branches, []string{defaultBranchName}) {
		t.Fatalf("branches = %#v, want only default branch", resultList.Branches)
	}
}

func TestDeleteBranchDiscardsStagingBeforeRecreation(t *testing.T) {
	repo := NewSeedRepository()
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: defaultBranchName}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "feature",
		Operations: []MutationOperation{
			{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"},
		},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if _, err := repo.DeleteBranch("feature"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if _, exists := repo.stagedMutations["feature"]; exists {
		t.Fatal("deleted branch retains staged mutations")
	}
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: defaultBranchName}); err != nil {
		t.Fatalf("recreate branch: %v", err)
	}

	status, err := repo.BranchStagingStatus("feature")
	if err != nil {
		t.Fatalf("BranchStagingStatus: %v", err)
	}
	if status != (BranchStagingStatus{Branch: "feature"}) {
		t.Fatalf("status = %#v", status)
	}
}

func TestDeleteBranchRejectsDefaultBeforeExistenceCheck(t *testing.T) {
	repo := NewSeedRepository()
	delete(repo.branches, defaultBranchName)

	if _, err := repo.DeleteBranch(defaultBranchName); !errors.Is(err, branch.ErrDefaultProtected) {
		t.Fatalf("DeleteBranch error = %v, want ErrDefaultProtected", err)
	}
}

func TestDeleteBranchRejectsMissingNonDefaultBranch(t *testing.T) {
	repo := NewSeedRepository()
	originalMain := repo.branches[defaultBranchName]

	if _, err := repo.DeleteBranch("missing"); !errors.Is(err, branch.ErrNotFound) {
		t.Fatalf("DeleteBranch error = %v, want ErrNotFound", err)
	}

	if got := repo.branches[defaultBranchName]; got != originalMain {
		t.Fatalf("default branch = %q, want unchanged %q", got, originalMain)
	}
}

func TestSwitchBranchMakesExistingInactiveBranchActive(t *testing.T) {
	repo := NewSeedRepository()
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: defaultBranchName}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	result, err := repo.SwitchBranch("feature")
	if err != nil {
		t.Fatalf("SwitchBranch: %v", err)
	}
	if result != (branch.SwitchResult{ActiveBranch: "feature"}) {
		t.Fatalf("result = %#v", result)
	}
	if got := repo.activeBranch; got != "feature" {
		t.Fatalf("active branch = %q, want feature", got)
	}
}

func TestSwitchBranchRejectsMissingBranchWithoutChangingActiveBranch(t *testing.T) {
	repo := NewSeedRepository()
	originalActiveBranch := repo.activeBranch
	originalBranches := make(map[string]ObjectID, len(repo.branches))
	for name, commitID := range repo.branches {
		originalBranches[name] = commitID
	}

	if _, err := repo.SwitchBranch("missing"); !errors.Is(err, branch.ErrNotFound) {
		t.Fatalf("SwitchBranch error = %v, want ErrNotFound", err)
	}
	if got := repo.activeBranch; got != originalActiveBranch {
		t.Fatalf("active branch = %q, want unchanged %q", got, originalActiveBranch)
	}
	if !reflect.DeepEqual(repo.branches, originalBranches) {
		t.Fatal("missing branch switch changed branch refs")
	}
}

func TestDeleteBranchRejectsActiveNonDefaultBranch(t *testing.T) {
	repo := NewSeedRepository()
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: defaultBranchName}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := repo.SwitchBranch("feature"); err != nil {
		t.Fatalf("SwitchBranch: %v", err)
	}

	if _, err := repo.DeleteBranch("feature"); !errors.Is(err, branch.ErrActiveProtected) {
		t.Fatalf("DeleteBranch error = %v, want ErrActiveProtected", err)
	}
	if got := repo.activeBranch; got != "feature" {
		t.Fatalf("active branch = %q, want feature", got)
	}
	if _, exists := repo.branches["feature"]; !exists {
		t.Fatal("active branch was deleted")
	}
}
