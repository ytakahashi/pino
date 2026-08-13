package application

import (
	"slices"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// How the folded set follows an edit, and what it drops when it cannot.

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
	for _, pointer := range pointers {
		p, err := domain.ParsePointer(pointer)
		if err != nil {
			t.Fatalf("ParsePointer(%q): %v", pointer, err)
		}

		v.Collapse(p)
	}

	return v
}

// pathAt is the path a pointer names.
func pathAt(t *testing.T, pointer string) domain.Path {
	t.Helper()

	p, err := domain.ParsePointer(pointer)
	if err != nil {
		t.Fatalf("ParsePointer(%q): %v", pointer, err)
	}

	return p
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

func TestApplyMovesTheFoldsWithThePathsThatMoved(t *testing.T) {
	t.Parallel()

	root := edited(t)
	v := foldedState(t, "/features/1", "/features/1/opt", "/features/2", "/server")

	res, err := domain.Insert(root, pathAt(t, "/features"), 1, domain.Member{
		Value: domain.NewNull(),
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	v.Apply(res)

	want := []string{"/features/2", "/features/2/opt", "/features/3", "/server"}
	if got := foldsOf(v); !slices.Equal(got, want) {
		t.Errorf("folded %v, want %v", got, want)
	}

	if !v.Cursor.Equal(res.Cursor) {
		t.Errorf("Cursor = %q, want %q", v.Cursor, res.Cursor)
	}
}

func TestApplyDropsTheFoldsInsideWhatWasDeleted(t *testing.T) {
	t.Parallel()

	root := edited(t)

	// The element that goes is folded, and so is the one that will take its
	// place. Following by resolution alone would leave the first pointer in
	// the set, where it would fold the element that moved up.
	v := foldedState(t, "/features/1", "/features/1/opt", "/features/2")

	res, err := domain.Delete(root, pathAt(t, "/features/1"))
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	v.Apply(res)

	want := []string{"/features/1"}
	if got := foldsOf(v); !slices.Equal(got, want) {
		t.Errorf("folded %v, want %v", got, want)
	}
}

func TestApplyDropsWhatWasRemovedBeforeMovingWhatSurvived(t *testing.T) {
	t.Parallel()

	root := edited(t)

	// Only the element that follows the deleted one is folded. It moves into
	// the deleted one's place, so dropping the removed paths last would take
	// it with them.
	v := foldedState(t, "/features/2")

	res, err := domain.Delete(root, pathAt(t, "/features/1"))
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	v.Apply(res)

	want := []string{"/features/1"}
	if got := foldsOf(v); !slices.Equal(got, want) {
		t.Errorf("folded %v, want %v", got, want)
	}
}

func TestApplyLeavesAKeyThatMerelyBeginsTheSameWay(t *testing.T) {
	t.Parallel()

	inner, err := domain.NewObject(nil)
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	root, err := domain.NewObject([]domain.Member{
		{Key: "a", Value: inner},
		{Key: "ab", Value: inner},
	})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	v := foldedState(t, "/a", "/ab")

	res, err := domain.Rename(root, pathAt(t, "/a"), "z")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	v.Apply(res)

	want := []string{"/ab", "/z"}
	if got := foldsOf(v); !slices.Equal(got, want) {
		t.Errorf("folded %v, want %v", got, want)
	}
}

func TestApplyDropsTheFoldsUnderAContainerThatLostItsChildren(t *testing.T) {
	t.Parallel()

	root := edited(t)
	v := foldedState(t, "/server", "/server/host", "/features/0")

	res, err := domain.ChangeType(root, pathAt(t, "/server"), domain.KindString)
	if err != nil {
		t.Fatalf("ChangeType: %v", err)
	}

	v.Apply(res)

	// /server itself goes too: what is folded there is not the container that
	// was folded.
	want := []string{"/features/0"}
	if got := foldsOf(v); !slices.Equal(got, want) {
		t.Errorf("folded %v, want %v", got, want)
	}
}

func TestRetainDropsOnlyWhatTheTreeNoLongerHas(t *testing.T) {
	t.Parallel()

	root := edited(t)
	v := foldedState(t, "/features/0", "/server", "/server/gone", "/nowhere")

	v.Retain(root)

	want := []string{"/features/0", "/server"}
	if got := foldsOf(v); !slices.Equal(got, want) {
		t.Errorf("folded %v, want %v", got, want)
	}
}

func TestFoldingEverythingAndThenEditingLeavesTheSetResolvable(t *testing.T) {
	t.Parallel()

	root := edited(t)

	v := NewViewState()
	v.CollapseAll(root)

	res, err := domain.Delete(root, pathAt(t, "/features/0"))
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	v.Apply(res)

	// The invariant, checked against the set the document was folded down to
	// rather than against pointers chosen by hand.
	for _, pointer := range foldsOf(v) {
		if _, ok := domain.Resolve(res.Root, pathAt(t, pointer)); !ok {
			t.Errorf("%q is folded but does not resolve", pointer)
		}
	}

	if len(foldsOf(v)) == 0 {
		t.Error("the whole set was dropped, so this checks nothing")
	}
}
