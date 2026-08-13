package application

import (
	"slices"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// foldsOf is the pointers of the folded set, in order, so a test can say what
// it expects as a list.
func foldsOf(v ViewState) []string {
	out := make([]string, 0, len(v.Collapsed))
	for pointer := range v.Collapsed {
		out = append(out, pointer)
	}

	slices.Sort(out)

	return out
}

// foldedState is a view state folded at the given pointers.
func foldedState(t *testing.T, pointers ...string) ViewState {
	t.Helper()

	v := NewViewState()
	for _, ptr := range pointers {
		v.Collapse(pointer(t, ptr))
	}

	return v
}

// edited is a document with an array of containers, which is where a folded
// set goes stale in the most ways at once.
//
//	{
//	  "features": [ {"opt": {}}, {"opt": {}}, {"opt": {}} ],
//	  "server":   { "host": {} }
//	}
func edited(t *testing.T) domain.Node {
	t.Helper()

	element := func() domain.Node {
		inner, err := domain.NewObject(nil)
		if err != nil {
			t.Fatalf("NewObject: %v", err)
		}

		outer, err := domain.NewObject([]domain.Member{{Key: "opt", Value: inner}})
		if err != nil {
			t.Fatalf("NewObject: %v", err)
		}

		return outer
	}

	host, err := domain.NewObject(nil)
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	server, err := domain.NewObject([]domain.Member{{Key: "host", Value: host}})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	root, err := domain.NewObject([]domain.Member{
		{Key: "features", Value: domain.NewArray([]domain.Node{element(), element(), element()})},
		{Key: "server", Value: server},
	})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	return root
}
