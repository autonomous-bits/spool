package repository

import "testing"

const (
	benchmarkNodeCount = 128
	benchmarkEdgeCount = 512
)

func BenchmarkRepositoryLifecycle(b *testing.B) {
	operations := stressGraphOperations(stressConfig{
		nodeCount: benchmarkNodeCount, edgeCount: benchmarkEdgeCount,
	})

	b.Run("mutation-normalization-candidate-construction", func(b *testing.B) {
		repo := benchmarkInitializedRepository(b)
		b.Cleanup(func() { closeBenchmarkRepository(b, repo) })
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
				b.Fatalf("StageMutationBatch: %v", err)
			}
		}
	})

	b.Run("immutable-persistence-and-ref-publication", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			b.StopTimer()
			repo := benchmarkInitializedRepository(b)
			if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
				b.Fatalf("StageMutationBatch: %v", err)
			}
			b.StartTimer()
			if _, err := repo.CommitStagedMutations("main"); err != nil {
				b.Fatalf("CommitStagedMutations: %v", err)
			}
			b.StopTimer()
			closeBenchmarkRepository(b, repo)
		}
	})

	b.Run("repository-reopen-and-control-state-loading", func(b *testing.B) {
		stateDir, _ := benchmarkCommittedRepository(b, operations)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			repo, err := OpenRepository(stateDir)
			if err != nil {
				b.Fatalf("OpenRepository: %v", err)
			}
			if err := repo.Close(); err != nil {
				b.Fatalf("Close: %v", err)
			}
		}
	})

	b.Run("projection-reconstruction", func(b *testing.B) {
		stateDir, head := benchmarkCommittedRepository(b, operations)
		repo, err := OpenRepository(stateDir)
		if err != nil {
			b.Fatalf("OpenRepository: %v", err)
		}
		b.Cleanup(func() { closeBenchmarkRepository(b, repo) })
		snapshotID := repo.commits[head].Snapshot
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			b.StopTimer()
			repo.mu.Lock()
			repo.evictSnapshotProjectionLocked(snapshotID)
			b.StartTimer()
			err := repo.ensureSnapshotProjectionLocked(snapshotID)
			repo.mu.Unlock()
			if err != nil {
				b.Fatalf("ensureSnapshotProjectionLocked: %v", err)
			}
		}
	})

	b.Run("gc-packing-and-publication", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			b.StopTimer()
			repo := benchmarkInitializedRepository(b)
			if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
				b.Fatalf("StageMutationBatch: %v", err)
			}
			if _, err := repo.CommitStagedMutations("main"); err != nil {
				b.Fatalf("CommitStagedMutations: %v", err)
			}
			b.StartTimer()
			if _, err := repo.GC(GCOptions{}); err != nil {
				b.Fatalf("GC: %v", err)
			}
			b.StopTimer()
			closeBenchmarkRepository(b, repo)
		}
	})

	b.Run("packed-repository-reopen", func(b *testing.B) {
		stateDir, _ := benchmarkCommittedRepository(b, operations)
		repo, err := OpenRepository(stateDir)
		if err != nil {
			b.Fatalf("OpenRepository before GC: %v", err)
		}
		if _, err := repo.GC(GCOptions{}); err != nil {
			b.Fatalf("GC: %v", err)
		}
		closeBenchmarkRepository(b, repo)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			reopened, err := OpenRepository(stateDir)
			if err != nil {
				b.Fatalf("OpenRepository packed: %v", err)
			}
			if err := reopened.Close(); err != nil {
				b.Fatalf("Close packed repository: %v", err)
			}
		}
	})
}

func benchmarkInitializedRepository(b *testing.B) *Repository {
	b.Helper()
	repo, err := InitializeRepository(b.TempDir())
	if err != nil {
		b.Fatalf("InitializeRepository: %v", err)
	}
	return repo
}

func benchmarkCommittedRepository(b *testing.B, operations []MutationOperation) (string, ObjectID) {
	b.Helper()
	repo := benchmarkInitializedRepository(b)
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
		b.Fatalf("StageMutationBatch: %v", err)
	}
	result, err := repo.CommitStagedMutations("main")
	if err != nil {
		b.Fatalf("CommitStagedMutations: %v", err)
	}
	stateDir := repo.mergeStateDir
	closeBenchmarkRepository(b, repo)
	return stateDir, result.Commit
}

func closeBenchmarkRepository(b *testing.B, repo *Repository) {
	b.Helper()
	if err := repo.Close(); err != nil {
		b.Fatalf("Close: %v", err)
	}
}
