package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// Folding the whole document at once, and what becomes of a selection that
// was inside what closed.

func TestCollapseAllLeavesTheRootOpenAndFoldsItsMembers(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	app.Do(ActionCollapseAll{})

	// The root stays open, so its members are what is left on screen.
	want := []string{"", "/name", "/server", "/debug"}
	got := pointersOf(app)

	if len(got) != len(want) {
		t.Fatalf("a folded document shows %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d is %q, want %q", i, got[i], want[i])
		}
	}

	if app.view.IsCollapsed(domain.Path{}) {
		t.Error("folding everything folded the root as well")
	}
}

// Folding everything folds what is inside the containers too, so unfolding one
// reveals its members already closed. Descending is then a level at a time.
func TestCollapseAllFoldsNestedContainers(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	app.Do(ActionCollapseAll{})

	if !app.view.IsCollapsed(path(domain.KeySegment("server"), domain.KeySegment("ports"))) {
		t.Fatal("a container inside a folded one was left open")
	}

	// Unfold /server: its array member appears, still folded.
	press(app, ActionMoveNext{}, ActionMoveNext{}, ActionMoveIn{})

	if got := cursorOf(app); got != "/server" {
		t.Fatalf("expected to be on /server, got %q", got)
	}

	for _, l := range app.Frame().Lines {
		if l.Path.String() == "/server/ports" && !l.Collapsed {
			t.Error("the array inside is drawn open, want it still folded")
		}

		if l.Path.String() == "/server/ports/0" {
			t.Error("the elements of the folded array are on screen")
		}
	}
}

// A container with nothing in it says as much either way, and folding the row
// it is drawn on would only offer to unfold into nothing.
func TestCollapseAllLeavesEmptyContainers(t *testing.T) {
	t.Parallel()

	inside, err := domain.NewComment(" preserved", false, true)
	if err != nil {
		t.Fatalf("NewComment: %v", err)
	}
	commentedObject := domain.WithTrivia(object(t), domain.NewTrivia(nil, nil, []domain.Comment{inside}))
	commentedArray := domain.WithTrivia(domain.NewArray(nil), domain.NewTrivia(nil, nil, []domain.Comment{inside}))

	app := session(t, object(t,
		member("opts", object(t)),
		member("tags", domain.NewArray(nil)),
		member("commented-object", commentedObject),
		member("commented-array", commentedArray),
		member("server", object(t, member("host", text(t, "localhost")))),
	))

	app.Do(ActionCollapseAll{})

	for _, key := range []string{"opts", "tags"} {
		if app.view.IsCollapsed(path(domain.KeySegment(key))) {
			t.Errorf("/%s is empty but was folded", key)
		}
	}

	if !app.view.IsCollapsed(path(domain.KeySegment("server"))) {
		t.Error("/server has members but was left open")
	}

	for _, key := range []string{"commented-object", "commented-array"} {
		if !app.view.IsCollapsed(path(domain.KeySegment(key))) {
			t.Errorf("/%s has an inside comment but was left open", key)
		}
	}
}

// A document that is a single value has no container to fold.
func TestCollapseAllLeavesAScalarDocumentAlone(t *testing.T) {
	t.Parallel()

	app := session(t, text(t, "only"))
	before := len(app.Frame().Lines)

	app.Do(ActionCollapseAll{})

	if got := len(app.Frame().Lines); got != before {
		t.Errorf("the document draws %d rows after folding, want %d", got, before)
	}
}

// Folding everything leaves the cursor inside something no longer drawn, and
// it has to come back to the nearest thing that is.
func TestCollapseAllRecoversTheCursor(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	press(app, ActionMoveIn{}, ActionMoveNext{}, ActionMoveIn{}, ActionMoveNext{}, ActionMoveIn{})

	if got := cursorOf(app); got != "/server/ports/0" {
		t.Fatalf("expected to be on /server/ports/0, got %q", got)
	}

	app.Do(ActionCollapseAll{})

	if got := cursorOf(app); got != "/server" {
		t.Errorf("the cursor settled on %q, want the nearest node still drawn", got)
	}
}

// A cursor on a container that is folded stays where it is: that row is still
// drawn, as the one line the container has become.
func TestCollapseAllKeepsACursorOnAContainer(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	press(app, ActionMoveNext{}, ActionMoveNext{}) // /server

	app.Do(ActionCollapseAll{})

	if got := cursorOf(app); got != "/server" {
		t.Errorf("the cursor moved to %q, want it to stay on /server", got)
	}
}

func TestExpandAllRestoresEveryContainer(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	before := len(app.Frame().Lines)

	app.Do(ActionCollapseAll{})

	if got := len(app.Frame().Lines); got >= before {
		t.Fatalf("folding everything left %d rows, want fewer than %d", got, before)
	}

	app.Do(ActionExpandAll{})

	if got := len(app.Frame().Lines); got != before {
		t.Errorf("unfolding everything left %d rows, want the original %d", got, before)
	}

	if got := len(app.view.Collapsed); got != 0 {
		t.Errorf("%d nodes are still folded", got)
	}

	// Folding one container by hand and then unfolding everything clears that
	// too, not only what folding everything had added.
	press(app, ActionMoveNext{}, ActionMoveNext{}, ActionMoveOut{})

	if !app.view.IsCollapsed(path(domain.KeySegment("server"))) {
		t.Fatal("the fixture did not fold")
	}

	app.Do(ActionExpandAll{})

	if got := len(app.Frame().Lines); got != before {
		t.Errorf("unfolding left %d rows, want the original %d", got, before)
	}
}

func TestFoldAllDoesNothingWithoutADocument(t *testing.T) {
	t.Parallel()

	for name, act := range map[string]Action{
		"collapse": ActionCollapseAll{},
		"expand":   ActionExpandAll{},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := New(Deps{JSONView: documentview.NewJSONRenderer(), TreeView: documentview.NewTreeRenderer()}, Config{})

			if effects := app.Do(act); effects != nil {
				t.Errorf("Do() = %v with no document open, want none", effects)
			}

			if frame := app.Frame(); frame.Cursor != -1 || frame.Scroll != 0 {
				t.Errorf("Frame() = %+v with no document open", frame)
			}
		})
	}
}
