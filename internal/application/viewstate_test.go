package application

import (
	"slices"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// How the folded set follows an edit, and what it drops when it cannot.

func TestApplyMovesTheFoldsWithThePathsThatMoved(t *testing.T) {
	t.Parallel()

	root := edited(t)
	v := foldedState(t, "/features/1", "/features/1/opt", "/features/2", "/server")

	res, err := domain.Insert(root, pointer(t, "/features"), 1, domain.Member{
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

	res, err := domain.Delete(root, pointer(t, "/features/1"))
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

	res, err := domain.Delete(root, pointer(t, "/features/1"))
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

	res, err := domain.Rename(root, pointer(t, "/a"), "z")
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

	res, err := domain.ChangeType(root, pointer(t, "/server"), domain.KindString)
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

	res, err := domain.Delete(root, pointer(t, "/features/0"))
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	v.Apply(res)

	// The invariant, checked against the set the document was folded down to
	// rather than against pointers chosen by hand.
	for _, ptr := range foldsOf(v) {
		if _, ok := domain.Resolve(res.Root, pointer(t, ptr)); !ok {
			t.Errorf("%q is folded but does not resolve", ptr)
		}
	}

	if len(foldsOf(v)) == 0 {
		t.Error("the whole set was dropped, so this checks nothing")
	}
}

// The view state is what carries the limit to the renderer, so a document
// opened without anyone choosing one still has values shortened.
func TestNewViewStateShortensLongValues(t *testing.T) {
	t.Parallel()

	if got := NewViewState().RenderOptions().MaxStrLen; got <= 0 {
		t.Errorf("MaxStrLen = %d, want a limit; long values would be drawn in full", got)
	}
}
