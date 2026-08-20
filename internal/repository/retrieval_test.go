package repository

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestProjectionRetrievalSearchAndMetadataFilters(t *testing.T) {
	repo, head := projectionRetrievalRepository(t)

	search, err := repo.SearchNodes(SearchNodesRequest{
		Branch: "main", Commit: head, Query: "bodyonly", MaxRows: 10, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("SearchNodes: %v", err)
	}
	if got, want := search.Commit, head; got != want {
		t.Fatalf("search commit = %q, want %q", got, want)
	}
	if len(search.Matches) != 2 {
		t.Fatalf("search matches = %#v, want two body matches", search.Matches)
	}
	if search.ContinuationToken != "" {
		t.Fatalf("complete search returned continuation %q", search.ContinuationToken)
	}
	for _, match := range search.Matches {
		if !reflect.DeepEqual(match.MatchedFields, []string{"body"}) {
			t.Fatalf("matched fields for %s = %#v, want body", match.Node.ID, match.MatchedFields)
		}
		if snippet := match.Snippets["body"]; snippet == "" || !containsAll(snippet, "<mark>", "bodyonly") {
			t.Fatalf("body snippet for %s = %q", match.Node.ID, snippet)
		}
	}

	rank := 3.0
	filtered, err := repo.FilterNodes(FilterNodesRequest{
		Branch: "main", Commit: head, Labels: []string{"Task"},
		Predicates: []MetadataPredicate{{Key: "rank", NumberEquals: &rank}},
		MaxRows:    10, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("FilterNodes numeric equality: %v", err)
	}
	if got, want := nodeIDs(filtered.Nodes), []string{"node-a", "node-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered node IDs = %#v, want %#v", got, want)
	}
	if filtered.ContinuationToken != "" {
		t.Fatalf("complete filter returned continuation %q", filtered.ContinuationToken)
	}
	if filtered.Nodes[0].Properties["rank"].Kind != PropertyInteger || filtered.Nodes[0].Properties["rank"].Integer != 3 {
		t.Fatalf("filtered node lost typed properties: %#v", filtered.Nodes[0])
	}

	min, max := 3.0, 7.0
	ranged, err := repo.FilterNodes(FilterNodesRequest{
		Branch: "main", Commit: head,
		Predicates: []MetadataPredicate{{Key: "rank", NumberMin: &min, NumberMax: &max}},
		MaxRows:    10, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("FilterNodes numeric range: %v", err)
	}
	if got, want := nodeIDs(ranged.Nodes), []string{SeedNodeID, "node-a", "node-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("range node IDs = %#v, want %#v", got, want)
	}

	text := "bodyonly"
	textFiltered, err := repo.FilterNodes(FilterNodesRequest{
		Branch: "main", Commit: head, Predicates: []MetadataPredicate{{Key: "search", TextEquals: &text}},
		MaxRows: 10, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("FilterNodes text equality: %v", err)
	}
	if got, want := nodeIDs(textFiltered.Nodes), []string{"node-a", "node-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("text node IDs = %#v, want %#v", got, want)
	}
}

func TestProjectionRetrievalEnforcesProjectionAndQueryBounds(t *testing.T) {
	repo, head := projectionRetrievalRepository(t)
	previous := repo.commits[head].Parents[0]
	rank := 3.0
	text := "bodyonly"

	for _, test := range []struct {
		name string
		call func() error
		want error
	}{
		{
			name: "historical pinned commit",
			call: func() error {
				_, err := repo.SearchNodes(SearchNodesRequest{
					Branch: "main", Commit: previous, Query: "bodyonly", MaxRows: 1, MaxResponseBytes: 1024,
				})
				return err
			},
			want: ErrHistoricalProjectionUnsupported,
		},
		{
			name: "unindexed property",
			call: func() error {
				_, err := repo.FilterNodes(FilterNodesRequest{
					Branch: "main", Commit: head, Predicates: []MetadataPredicate{{Key: "hidden", TextEquals: &text}},
					MaxRows: 1, MaxResponseBytes: 1024,
				})
				return err
			},
			want: ErrUnindexedMetadataProperty,
		},
		{
			name: "unsupported property type",
			call: func() error {
				_, err := repo.FilterNodes(FilterNodesRequest{
					Branch: "main", Commit: head, Predicates: []MetadataPredicate{{Key: "rank", TextEquals: &text}},
					MaxRows: 1, MaxResponseBytes: 1024,
				})
				return err
			},
			want: ErrUnsupportedMetadataPredicate,
		},
		{
			name: "invalid budget",
			call: func() error {
				_, err := repo.FilterNodes(FilterNodesRequest{
					Branch: "main", Commit: head, Predicates: []MetadataPredicate{{Key: "rank", NumberEquals: &rank}},
					MaxRows: 0, MaxResponseBytes: 1024,
				})
				return err
			},
			want: ErrInvalidListBudget,
		},
		{
			name: "response budget",
			call: func() error {
				_, err := repo.SearchNodes(SearchNodesRequest{
					Branch: "main", Commit: head, Query: "bodyonly", MaxRows: 1, MaxResponseBytes: 1,
				})
				return err
			},
			want: ErrResponseBudgetTooSmall,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}

	first, err := repo.SearchNodes(SearchNodesRequest{
		Branch: "main", Commit: head, Query: "bodyonly", MaxRows: 1, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("first search page: %v", err)
	}
	if len(first.Matches) != 1 || first.ContinuationToken == "" {
		t.Fatalf("first page = %#v, want a continuation", first)
	}
	second, err := repo.SearchNodes(SearchNodesRequest{
		Branch: "main", Commit: head, Query: "bodyonly", MaxRows: 1, MaxResponseBytes: 1 << 20,
		ContinuationToken: first.ContinuationToken,
	})
	if err != nil {
		t.Fatalf("second search page: %v", err)
	}
	if len(second.Matches) != 1 || second.Matches[0].Node.ID == first.Matches[0].Node.ID || second.ContinuationToken != "" {
		t.Fatalf("second page = %#v", second)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repo.SearchNodesContext(ctx, SearchNodesRequest{
		Branch: "main", Commit: head, Query: "bodyonly", MaxRows: 1, MaxResponseBytes: 1024,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("SearchNodesContext cancellation error = %v", err)
	}
	if _, err := repo.FilterNodesContext(ctx, FilterNodesRequest{
		Branch: "main", Commit: head, Predicates: []MetadataPredicate{{Key: "rank", NumberEquals: &rank}},
		MaxRows: 1, MaxResponseBytes: 1024,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("FilterNodesContext cancellation error = %v", err)
	}
}

func projectionRetrievalRepository(t *testing.T) (*Repository, ObjectID) {
	t.Helper()
	repo, err := InitializeRepository(t.TempDir())
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)
	schema := []byte(`
version = 2
[[node]]
label = "Seed"
[[node.property]]
key = "search"
required = true
indexed = true
types = ["string"]
[[node.property]]
key = "rank"
required = true
indexed = true
types = ["integer"]
[[node.property]]
key = "hidden"
required = true
types = ["string"]
[[node]]
label = "Task"
[[node.property]]
key = "search"
required = true
indexed = true
types = ["string"]
[[node.property]]
key = "rank"
required = true
indexed = true
types = ["integer"]
`)
	_, err = repo.StageSchemaMigration(SchemaMigrationRequest{
		Branch: "main", SchemaTOML: schema,
		Operations: []MutationOperation{
			{
				Action: "update", Entity: "node", ID: SeedNodeID, Title: "Seed document", Labels: []string{"Seed"},
				Properties: map[string]PropertyValue{
					"search": StringPropertyValue("seed material"),
					"rank":   IntegerPropertyValue(7),
					"hidden": StringPropertyValue("not indexed"),
				},
			},
			{
				Action: "add", Entity: "node", ID: "node-a", Title: "First task", Labels: []string{"Task"},
				Properties: map[string]PropertyValue{
					"search": StringPropertyValue("bodyonly"),
					"rank":   IntegerPropertyValue(3),
				},
			},
			{
				Action: "add", Entity: "node", ID: "node-b", Title: "Second task", Labels: []string{"Task"},
				Properties: map[string]PropertyValue{
					"search": StringPropertyValue("bodyonly"),
					"rank":   IntegerPropertyValue(3),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("StageSchemaMigration: %v", err)
	}
	committed, err := repo.CommitStagedMutationBatch(CommitStagedMutationRequest{Branch: "main"})
	if err != nil {
		t.Fatalf("CommitStagedMutationBatch: %v", err)
	}
	return repo, committed.Commit
}

func nodeIDs(nodes []Node) []string {
	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = node.ID
	}
	return ids
}

func containsAll(value string, values ...string) bool {
	for _, expected := range values {
		if !strings.Contains(value, expected) {
			return false
		}
	}
	return true
}
