package resolve

import (
	"context"
	"testing"
	"time"
)

func BenchmarkRetrievalFixtureConstruction(b *testing.B) {
	config := retrievalStressConfig{nodeCount: 128, edgeCount: 512}
	b.ReportAllocs()
	for b.Loop() {
		repo, _ := retrievalStressRepository(b, config)
		if err := repo.Close(); err != nil {
			b.Fatalf("Close: %v", err)
		}
	}
}

func BenchmarkRetrievalQueries(b *testing.B) {
	config := retrievalStressConfig{nodeCount: 128, edgeCount: 512}
	repo, head := retrievalStressRepository(b, config)
	b.Cleanup(func() {
		if err := repo.Close(); err != nil {
			b.Errorf("Close: %v", err)
		}
	})
	budget := QueryBudget{
		MaxRows: 127, MaxResponseBytes: 1 << 20, MaxDepth: 1, MaxVisited: 256, Timeout: time.Minute,
	}
	tool := NewResolveToolWithOptions(repo, Options{QueryBudget: &budget})
	selector := SnapshotSelector{Branch: "main"}

	b.Run("search", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := tool.SPLSearch(context.Background(), SearchRequest{Selector: selector, Query: "querycorpus"}); err != nil {
				b.Fatalf("SPLSearch: %v", err)
			}
		}
	})
	b.Run("filter", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := tool.SPLFilter(context.Background(), FilterRequest{Selector: selector, Labels: []string{"Task"}}); err != nil {
				b.Fatalf("SPLFilter: %v", err)
			}
		}
	})
	b.Run("search-expand", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := tool.SPLSearchExpand(context.Background(), SearchExpandRequest{
				Selector: selector, Seeds: SeedSelector{Query: "queryroot"}, Direction: DirectionOut, EdgeTypes: []string{"STRESS_LINK"},
			}); err != nil {
				b.Fatalf("SPLSearchExpand: %v", err)
			}
		}
	})
	b.Run("context", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := tool.SPLContext(context.Background(), ContextRequest{
				Selector: selector, Seeds: SeedSelector{Labels: []string{"Seed"}}, Direction: DirectionOut, EdgeTypes: []string{"STRESS_LINK"},
			}); err != nil {
				b.Fatalf("SPLContext: %v", err)
			}
		}
	})
	if head == "" {
		b.Fatal("retrieval fixture did not create a commit")
	}
}
