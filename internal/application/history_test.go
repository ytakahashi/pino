package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// The versions of a document and the walk back and forth through them. What
// they hold is a whole tree, so nothing here needs a document to be open or an
// edit to have happened: a version is made of a root and a place to stand.

func TestHistoryWalksBackAndForthThroughTheVersions(t *testing.T) {
	t.Parallel()

	first := version(t, "open")
	second := version(t, "edit /a")
	third := version(t, "edit /b")

	h := NewHistory(first)
	h.Push(second)
	h.Push(third)

	rev, _, ok := h.Undo()
	if !ok || rev.Label != "edit /a" {
		t.Fatalf("undo gave %q (%v), want edit /a", rev.Label, ok)
	}

	if rev.Root != second.Root {
		t.Error("undo gave a version whose tree is not the one that was pushed")
	}

	rev, _, ok = h.Undo()
	if !ok || rev.Label != "open" {
		t.Fatalf("undo gave %q (%v), want open", rev.Label, ok)
	}

	rev, _, ok = h.Redo()
	if !ok || rev.Label != "edit /a" {
		t.Fatalf("redo gave %q (%v), want edit /a", rev.Label, ok)
	}

	rev, _, ok = h.Redo()
	if !ok || rev.Label != "edit /b" {
		t.Fatalf("redo gave %q (%v), want edit /b", rev.Label, ok)
	}
}

func TestHistoryReportsWhereTheChangeBeingToggledHappened(t *testing.T) {
	t.Parallel()

	first := version(t, "open")

	second := version(t, "edit /a")
	second.Cursor = domain.Path{}.Child(domain.KeySegment("a"))

	h := NewHistory(first)
	h.Push(second)

	// Undoing takes the second version away, so what the reader is watching is
	// where that version's edit happened — not where they stood in the first
	// one, which was wherever a document is opened at.
	_, at, ok := h.Undo()
	if !ok || at.String() != "/a" {
		t.Errorf("undo pointed at %q (%v), want /a", at, ok)
	}

	// Redoing puts the same change back, so it points at the same place.
	_, at, ok = h.Redo()
	if !ok || at.String() != "/a" {
		t.Errorf("redo pointed at %q (%v), want /a", at, ok)
	}
}

func TestHistoryStopsAtTheVersionTheDocumentWasOpenedAt(t *testing.T) {
	t.Parallel()

	h := NewHistory(version(t, "open"))
	h.Push(version(t, "edit /a"))

	if _, _, ok := h.Undo(); !ok {
		t.Fatal("undo refused to step back from the only edit")
	}

	if rev, _, ok := h.Undo(); ok {
		t.Errorf("undo went back past the version it was opened at, to %q", rev.Label)
	}
}

func TestHistoryStopsAtTheMostRecentVersion(t *testing.T) {
	t.Parallel()

	h := NewHistory(version(t, "open"))
	h.Push(version(t, "edit /a"))

	if rev, _, ok := h.Redo(); ok {
		t.Errorf("redo went past the most recent version, to %q", rev.Label)
	}
}

func TestPushingAfterUndoDropsWhatWasUndone(t *testing.T) {
	t.Parallel()

	h := NewHistory(version(t, "open"))
	h.Push(version(t, "edit /a"))
	h.Push(version(t, "edit /b"))

	if _, _, ok := h.Undo(); !ok {
		t.Fatal("undo refused")
	}

	// Editing from here makes the version that was undone a future that was
	// not taken.
	h.Push(version(t, "edit /c"))

	if rev, _, ok := h.Redo(); ok {
		t.Errorf("redo reached %q, want nowhere to go", rev.Label)
	}

	rev, _, ok := h.Undo()
	if !ok || rev.Label != "edit /a" {
		t.Errorf("undo gave %q (%v), want edit /a", rev.Label, ok)
	}
}

func TestAHistoryWithNothingInItHasNowhereToGo(t *testing.T) {
	t.Parallel()

	// The zero value is what a session with no document open holds, so
	// pressing u before opening anything comes through here.
	var h History

	if rev, _, ok := h.Undo(); ok {
		t.Errorf("undo gave %q, want nowhere to go", rev.Label)
	}

	if rev, _, ok := h.Redo(); ok {
		t.Errorf("redo gave %q, want nowhere to go", rev.Label)
	}

	// And it still takes a version, rather than failing on the empty slice it
	// would have to truncate.
	h.Push(version(t, "open"))

	if _, _, ok := h.Undo(); ok {
		t.Error("undo went back past the first version pushed")
	}
}
