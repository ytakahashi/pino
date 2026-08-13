package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// Going back and forth through the versions of an open document.
//
// What makes these tests short is that a version is a whole tree: undoing is
// choosing an earlier root, so "the document came back" is one pointer
// comparison and needs no notion of two trees being equal.

// commitEdit makes an edit current, the way the editing flow will once it is
// wired up: the new tree, a version to come back to, and the folded set
// following where things moved.
func commitEdit(t *testing.T, a *App, res domain.EditResult, label string) {
	t.Helper()

	a.doc.Replace(res.Root)
	a.history.Push(Revision{Root: res.Root, Cursor: res.Cursor, Label: label})
	a.view.Apply(res)
	a.settle(a.render())
}

func TestUndoBringsBackTheTreeThatWasThere(t *testing.T) {
	t.Parallel()

	// Every document of the corpus, so that the property is checked against a
	// growing set rather than against one shape chosen to satisfy it.
	for name, doc := range documents(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := session(t, doc.root)
			before := app.doc.Root()

			// Replacing the root is the one edit every document can take,
			// whatever it holds; none of the corpus is a boolean.
			res, err := domain.SetValue(before, domain.Path{}, domain.NewBool(true))
			if err != nil {
				t.Fatalf("SetValue: %v", err)
			}

			commitEdit(t, app, res, "edit /")

			if !app.doc.IsDirty() {
				t.Fatal("the document is not marked unsaved after an edit")
			}

			app.Do(ActionUndo{})

			if app.doc.Root() != before {
				t.Error("undo did not bring back the tree that was there")
			}

			if app.doc.IsDirty() {
				t.Error("the unsaved mark stayed on after undoing back to the tree that was read")
			}

			app.Do(ActionRedo{})

			if app.doc.Root() != res.Root {
				t.Error("redo did not bring back the edited tree")
			}

			if !app.doc.IsDirty() {
				t.Error("the unsaved mark went off after redoing the edit")
			}
		})
	}
}

func TestUndoStandsWhereTheVersionSays(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	// Edit deep in the document, then look somewhere else entirely.
	port := path(domain.KeySegment("server"), domain.KeySegment("ports"), domain.IndexSegment(0))

	res, err := domain.SetValue(app.doc.Root(), port, domain.NewNumber("9090"))
	if err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	commitEdit(t, app, res, "edit /server/ports/0")

	app.Do(ActionMoveFirst{})

	if cursorOf(app) != "" {
		t.Fatalf("the cursor is at %q, want the root", cursorOf(app))
	}

	// Undo shows what it undid rather than leaving the reader where they were
	// standing: a change to a part of the document nobody can see is a change
	// nobody can check.
	app.Do(ActionUndo{})

	if got := cursorOf(app); got != "/server/ports/0" {
		t.Errorf("the cursor is at %q, want /server/ports/0", got)
	}

	app.Do(ActionMoveFirst{})
	app.Do(ActionRedo{})

	if got := cursorOf(app); got != "/server/ports/0" {
		t.Errorf("after redo the cursor is at %q, want /server/ports/0", got)
	}
}

func TestUndoingAnInsertionStandsOnWhatHeldIt(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	ports := path(domain.KeySegment("server"), domain.KeySegment("ports"))

	res, err := domain.Insert(app.doc.Root(), ports, 2, domain.Member{
		Value: domain.NewNumber("9090"),
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	commitEdit(t, app, res, "add /server/ports/2")

	if got := cursorOf(app); got != "/server/ports/2" {
		t.Fatalf("the cursor is at %q after adding, want /server/ports/2", got)
	}

	app.Do(ActionUndo{})

	// The element the insertion selected is what undo took away, so there is
	// nothing left to stand on. The array that held it is the closest thing to
	// where the change happened, and beats being sent to the top.
	if got := cursorOf(app); got != "/server/ports" {
		t.Errorf("the cursor is at %q, want /server/ports", got)
	}
}

func TestUndoStopsAtTheDocumentAsItWasRead(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	opened := app.doc.Root()

	// More presses than there are versions, which is what holding the key
	// down amounts to.
	press(app, repeat(ActionUndo{}, 5)...)

	if app.doc.Root() != opened {
		t.Error("undoing past the first version changed the document")
	}

	if app.doc.IsDirty() {
		t.Error("undoing past the first version marked the document unsaved")
	}
}

func TestRedoStopsAtTheMostRecentVersion(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	res, err := domain.SetValue(app.doc.Root(), path(domain.KeySegment("debug")), domain.NewBool(true))
	if err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	commitEdit(t, app, res, "edit /debug")
	press(app, repeat(ActionRedo{}, 3)...)

	if app.doc.Root() != res.Root {
		t.Error("redoing past the most recent version changed the document")
	}
}

func TestUndoAndRedoDoNothingWithNoDocumentOpen(t *testing.T) {
	t.Parallel()

	// A session that has opened nothing has an empty history, which is what
	// keeps these from reaching a document that is not there.
	app := New(Deps{JSONView: NewJSONRenderer(), TreeView: NewTreeRenderer()})

	press(app, ActionUndo{}, ActionRedo{})

	if frame := app.Frame(); len(frame.Lines) != 0 {
		t.Errorf("a session with nothing open drew %d rows", len(frame.Lines))
	}
}

func TestOpeningAnotherDocumentStartsTheHistoryAgain(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	res, err := domain.SetValue(app.doc.Root(), path(domain.KeySegment("debug")), domain.NewBool(true))
	if err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	commitEdit(t, app, res, "edit /debug")

	if err := app.Open("a.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	opened := app.doc.Root()
	app.Do(ActionUndo{})

	// The edit belonged to the document that was open before. Undoing it here
	// would apply a version of one file to another.
	if app.doc.Root() != opened {
		t.Error("undo reached back into the document opened before this one")
	}
}

// featureList is an array of containers, which is the one shape where a fold
// has to move: deleting an element brings the ones after it down, and a fold
// sits on one of those.
//
//	{ "features": [ {"name": "first"}, {"name": "second"} ] }
func featureList(t *testing.T) domain.Node {
	t.Helper()

	return object(t,
		member("features", domain.NewArray([]domain.Node{
			object(t, member("name", text(t, "first"))),
			object(t, member("name", text(t, "second"))),
		})),
	)
}

func TestUndoDoesNotPutFoldsBack(t *testing.T) {
	t.Parallel()

	app := session(t, featureList(t))

	first := path(domain.KeySegment("features"), domain.IndexSegment(0))
	second := path(domain.KeySegment("features"), domain.IndexSegment(1))

	// Fold the second element, then delete the first one.
	app.view.Collapse(second)

	res, err := domain.Delete(app.doc.Root(), first)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	commitEdit(t, app, res, "delete /features/0")

	// The element that was folded moved down into the deleted one's place, and
	// the fold went with it. Without this the rest of the test would pass
	// against a fold that never moved at all.
	if !app.view.IsCollapsed(first) {
		t.Fatalf("the fold is at %v after the edit, want /features/0", foldsOf(app.view))
	}

	app.Do(ActionUndo{})

	// The tree comes back and the view does not: what is folded is how the
	// document is being looked at rather than what it contains. The fold is
	// therefore on the element that was restored into position 0, which is not
	// the one the reader folded. This is a decision, not something waiting to
	// be fixed.
	if !app.view.IsCollapsed(first) {
		t.Errorf("the fold is at %v after undo, want it left at /features/0", foldsOf(app.view))
	}

	if app.view.IsCollapsed(second) {
		t.Error("undo put the fold back where it was, which it is not meant to do")
	}
}
