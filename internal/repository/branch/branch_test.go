package branch

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeStore struct {
	request      CreateRequest
	result       CreateResult
	listResult   ListResult
	deleteResult DeleteResult
	switchResult SwitchResult
	err          error
	listed       bool
	deleted      string
	switched     string
}

func (s *fakeStore) CreateBranch(name string, source Source) (CreateResult, error) {
	s.request = CreateRequest{Name: name, Source: source}
	return s.result, s.err
}

func (s *fakeStore) ListBranches() (ListResult, error) {
	s.listed = true
	return s.listResult, s.err
}

func (s *fakeStore) DeleteBranch(name string) (DeleteResult, error) {
	s.deleted = name
	return s.deleteResult, s.err
}

func (s *fakeStore) SwitchBranch(name string) (SwitchResult, error) {
	s.switched = name
	return s.switchResult, s.err
}

func TestServiceCreatesBranchWithExplicitSource(t *testing.T) {
	store := &fakeStore{result: CreateResult{Name: "feature", Commit: "commit"}}
	service := NewService(store)

	result, err := service.Create(context.Background(), CreateRequest{
		Name:   "feature",
		Source: Source{Branch: "main"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if result != store.result {
		t.Fatalf("result = %#v, want %#v", result, store.result)
	}
	if store.request != (CreateRequest{Name: "feature", Source: Source{Branch: "main"}}) {
		t.Fatalf("store request = %#v", store.request)
	}
}

func TestServiceRejectsInvalidSourceBeforeCallingStore(t *testing.T) {
	testCases := []struct {
		name   string
		source Source
		want   error
	}{
		{name: "missing", want: ErrMissingSource},
		{name: "ambiguous", source: Source{Branch: "main", Commit: "commit"}, want: ErrAmbiguousSource},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			store := &fakeStore{}
			_, err := NewService(store).Create(context.Background(), CreateRequest{Source: testCase.source})
			if !errors.Is(err, testCase.want) {
				t.Fatalf("Create error = %v, want %v", err, testCase.want)
			}
			if store.request != (CreateRequest{}) {
				t.Fatalf("invalid request reached store: %#v", store.request)
			}
		})
	}
}

func TestServiceListsBranches(t *testing.T) {
	store := &fakeStore{listResult: ListResult{Branches: []string{"feature", "main"}}}

	result, err := NewService(store).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if !store.listed {
		t.Fatal("List did not call the store")
	}
	if !reflect.DeepEqual(result, store.listResult) {
		t.Fatalf("result = %#v, want %#v", result, store.listResult)
	}
}

func TestServiceDeletesBranch(t *testing.T) {
	store := &fakeStore{deleteResult: DeleteResult{Name: "feature"}}

	result, err := NewService(store).Delete(context.Background(), DeleteRequest{Name: "feature"})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if store.deleted != "feature" {
		t.Fatalf("deleted branch = %q, want feature", store.deleted)
	}
	if result != store.deleteResult {
		t.Fatalf("result = %#v, want %#v", result, store.deleteResult)
	}
}

func TestServiceSwitchesBranch(t *testing.T) {
	store := &fakeStore{switchResult: SwitchResult{ActiveBranch: "feature"}}

	result, err := NewService(store).Switch(context.Background(), SwitchRequest{Name: "feature"})
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if store.switched != "feature" {
		t.Fatalf("switched branch = %q, want feature", store.switched)
	}
	if result != store.switchResult {
		t.Fatalf("result = %#v, want %#v", result, store.switchResult)
	}
}

func TestServiceRejectsCanceledContextBeforeCallingStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &fakeStore{}
	service := NewService(store)

	if _, err := service.Create(ctx, CreateRequest{Source: Source{Branch: "main"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Create error = %v, want context.Canceled", err)
	}
	if _, err := service.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("List error = %v, want context.Canceled", err)
	}
	if _, err := service.Delete(ctx, DeleteRequest{Name: "feature"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete error = %v, want context.Canceled", err)
	}
	if _, err := service.Switch(ctx, SwitchRequest{Name: "feature"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Switch error = %v, want context.Canceled", err)
	}
	if store.request != (CreateRequest{}) || store.listed || store.deleted != "" || store.switched != "" {
		t.Fatalf("canceled request reached store: %#v", store)
	}
}
