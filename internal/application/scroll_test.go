package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// Moving the window: what the reader is shown as the selection travels past
// the top or the bottom of it.

func TestScrollHalfMovesByHalfAWindow(t *testing.T) {
	t.Parallel()

	// A document long enough to scroll through: ten members of two rows each.
	members := make([]domain.Member, 0, 10)
	for i := range 10 {
		members = append(members, member(
			string(rune('a'+i)),
			domain.NewArray([]domain.Node{domain.NewNumber("1")}),
		))
	}

	app := session(t, object(t, members...))
	app.Do(ActionResize{Height: 10})

	total := len(app.Frame().Lines)
	if total < 30 {
		t.Fatalf("the fixture draws %d rows, too few to scroll through", total)
	}

	// Down half a window at a time, with the cursor keeping its place on the
	// screen: the window and the cursor travel together.
	app.Do(ActionScrollHalfDown{})

	frame := app.Frame()
	if frame.Scroll != 5 {
		t.Errorf("Scroll = %d after half a screen, want 5", frame.Scroll)
	}

	if frame.Cursor-frame.Scroll != 0 {
		t.Errorf("the cursor sits %d rows into the window, want 0 as before",
			frame.Cursor-frame.Scroll)
	}

	// And again, from a cursor no longer at the top of the window.
	app.Do(ActionMoveNext{})
	before := app.Frame()

	app.Do(ActionScrollHalfDown{})
	after := app.Frame()

	if after.Scroll != before.Scroll+5 {
		t.Errorf("Scroll = %d, want %d", after.Scroll, before.Scroll+5)
	}

	// The cursor keeps its place on the screen, give or take the snap onto a
	// row it can occupy: half a screen on from here is the closing row of an
	// array, and the nearest node is one further down. Every closing run in
	// this document is a single row, so that is the whole of the drift.
	// Stepping node by node rather than counting rows would drift twice as
	// far, and further with every press.
	drift := (after.Cursor - after.Scroll) - (before.Cursor - before.Scroll)
	if drift < 0 || drift > 1 {
		t.Errorf("the cursor drifted %d rows down the window, want at most 1", drift)
	}

	// Back up the way it came.
	app.Do(ActionScrollHalfUp{})

	if got := app.Frame().Scroll; got != before.Scroll {
		t.Errorf("Scroll = %d after going back up, want %d", got, before.Scroll)
	}
}

// Turning a wheel moves the window. The selection comes along only when the
// window would otherwise leave it behind, since the status bar names the node
// it is on and naming one off the screen says nothing.
func TestScrollByMovesByTheRequestedRows(t *testing.T) {
	t.Parallel()

	const height = 5

	app := session(t, sample(t))
	app.Do(ActionResize{Height: height})

	// Down three, from the top: the window moves and the selection, which was
	// on the first row, comes to the top of it.
	app.Do(ActionScrollBy{Rows: 3})

	frame := app.Frame()
	if frame.Scroll != 3 {
		t.Fatalf("Scroll = %d, want 3", frame.Scroll)
	}

	if frame.Cursor != 3 {
		t.Errorf("the selection is on row %d, want the top of the window", frame.Cursor)
	}

	// Back up three, with the selection inside the window all the while: it
	// stays exactly where it was.
	before := cursorOf(app)

	app.Do(ActionScrollBy{Rows: -3})

	if got := app.Frame().Scroll; got != 0 {
		t.Errorf("Scroll = %d, want 0", got)
	}

	if got := cursorOf(app); got != before {
		t.Errorf("the selection moved to %q, want it left on %q", got, before)
	}
}

// A selection pushed off the bottom is brought back to the last row of the
// window that it can occupy, closing rows not among them.
func TestScrollByPullsTheCursorUp(t *testing.T) {
	t.Parallel()

	const height = 5

	app := session(t, sample(t))
	app.Do(ActionResize{Height: height})
	app.Do(ActionMoveLast{})

	if got := cursorOf(app); got != "/debug" {
		t.Fatalf("expected to be on the last node, got %q", got)
	}

	app.Do(ActionScrollBy{Rows: -4})

	frame := app.Frame()

	if frame.Cursor < frame.Scroll || frame.Cursor >= frame.Scroll+height {
		t.Fatalf("the selection is on row %d, outside the window [%d, %d)",
			frame.Cursor, frame.Scroll, frame.Scroll+height)
	}

	// The bottom row of the window is the array's closing bracket, so the
	// selection settles on the element above it rather than on the bracket.
	if got := cursorOf(app); got != "/server/ports/1" {
		t.Errorf("the selection came back to %q, want /server/ports/1", got)
	}
}

// The window stops at the ends of the document however hard the wheel is
// turned, and the selection stays on screen.
func TestScrollByStopsAtTheEnds(t *testing.T) {
	t.Parallel()

	const height = 5

	app := session(t, sample(t))
	app.Do(ActionResize{Height: height})

	press(app, repeat(ActionScrollBy{Rows: 3}, 10)...)

	frame := app.Frame()
	if frame.Scroll != len(frame.Lines)-height {
		t.Errorf("Scroll = %d at the bottom, want %d", frame.Scroll, len(frame.Lines)-height)
	}

	if frame.Cursor < frame.Scroll || frame.Cursor >= frame.Scroll+height {
		t.Errorf("the selection is on row %d, outside the window", frame.Cursor)
	}

	press(app, repeat(ActionScrollBy{Rows: -3}, 10)...)

	frame = app.Frame()
	if frame.Scroll != 0 {
		t.Errorf("Scroll = %d at the top, want 0", frame.Scroll)
	}

	// The selection is dragged only as far as staying on screen requires, so
	// winding back to the top leaves it at the bottom of the window rather
	// than carrying it to the first row.
	if frame.Cursor < 0 || frame.Cursor >= height {
		t.Errorf("the selection is on row %d, outside the window [0, %d)", frame.Cursor, height)
	}
}

// Without a window there is nothing to scroll, and nothing to be pushed out
// of either.
func TestScrollByDoesNothingWithoutAWindow(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	app.Do(ActionScrollBy{Rows: 3})

	if got := app.Frame().Scroll; got != 0 {
		t.Errorf("Scroll = %d with no window, want 0", got)
	}

	if got := cursorOf(app); got != "" {
		t.Errorf("the selection moved to %q with no window, want the root", got)
	}

	empty := New(Deps{JSONView: documentview.NewJSONRenderer(), TreeView: documentview.NewTreeRenderer()})
	empty.Do(ActionScrollBy{Rows: 3})

	if frame := empty.Frame(); frame.Cursor != -1 || frame.Scroll != 0 {
		t.Errorf("Frame() = %+v with no document open", frame)
	}
}

// A document that fits has nowhere to scroll to, and the selection is left
// alone rather than dragged to an edge.
func TestScrollByLeavesADocumentThatFitsAtTheTop(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	app.Do(ActionResize{Height: 100})
	press(app, ActionMoveNext{}, ActionMoveNext{})

	before := cursorOf(app)

	app.Do(ActionScrollBy{Rows: 3})

	if got := app.Frame().Scroll; got != 0 {
		t.Errorf("Scroll = %d on a document that fits, want 0", got)
	}

	if got := cursorOf(app); got != before {
		t.Errorf("the selection moved to %q, want it left on %q", got, before)
	}
}

// The rows closing whatever is still open come after the last node, and the
// cursor never lands on them. Reaching the end of a document has to look like
// reaching it, so the window goes to the bottom rather than only as far as the
// cursor obliges it to.
func TestReachingTheLastNodeShowsTheEnd(t *testing.T) {
	t.Parallel()

	const height = 5

	ways := map[string][]Action{
		"jumping to the end": {ActionMoveLast{}},
		"walking down":       repeat(ActionMoveNext{}, 20),
		"reading on":         repeat(ActionScrollHalfDown{}, 20),
	}

	for name, actions := range ways {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := session(t, sample(t))
			app.Do(ActionResize{Height: height})
			press(app, actions...)

			if got := cursorOf(app); got != "/debug" {
				t.Fatalf("selected %q, want the last node", got)
			}

			frame := app.Frame()
			if frame.Scroll+height < len(frame.Lines) {
				t.Errorf("the window shows rows [%d, %d) of %d, want it to reach the end",
					frame.Scroll, frame.Scroll+height, len(frame.Lines))
			}
		})
	}
}

// A document that fits is drawn from the top wherever the cursor is, so
// reaching the end of one does not scroll it.
func TestReachingTheLastNodeOfADocumentThatFitsKeepsItAtTheTop(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	app.Do(ActionResize{Height: 100})
	app.Do(ActionMoveLast{})

	if got := app.Frame().Scroll; got != 0 {
		t.Errorf("Scroll = %d, want the document drawn from the top", got)
	}
}

// Sending the window to the bottom is for standing at the end, not a place it
// stays: moving back up scrolls as little as it has to again.
func TestLeavingTheLastNodeScrollsBack(t *testing.T) {
	t.Parallel()

	const height = 5

	app := session(t, sample(t))
	app.Do(ActionResize{Height: height})
	app.Do(ActionMoveLast{})

	atEnd := app.Frame().Scroll

	press(app, repeat(ActionMovePrev{}, 20)...)

	if got := app.Frame().Scroll; got >= atEnd {
		t.Errorf("Scroll = %d after walking back to the top, want less than %d", got, atEnd)
	}
}

// At either end the window stops, and the cursor has to stop somewhere it can
// still be seen.
func TestScrollHalfStopsAtTheEnds(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	app.Do(ActionResize{Height: 4})

	for range 10 {
		app.Do(ActionScrollHalfDown{})
	}

	frame := app.Frame()
	if got := cursorOf(app); got != "/debug" {
		t.Errorf("scrolling to the bottom selected %q, want the last node", got)
	}

	if frame.Cursor < frame.Scroll || frame.Cursor >= frame.Scroll+4 {
		t.Errorf("cursor row %d is outside the window [%d, %d)",
			frame.Cursor, frame.Scroll, frame.Scroll+4)
	}

	for range 10 {
		app.Do(ActionScrollHalfUp{})
	}

	frame = app.Frame()
	if got := cursorOf(app); got != "" {
		t.Errorf("scrolling to the top selected %q, want the root", got)
	}

	if frame.Scroll != 0 {
		t.Errorf("Scroll = %d at the top, want 0", frame.Scroll)
	}
}

// A document that fits has nothing to scroll, but the cursor still travels.
func TestScrollHalfLeavesADocumentThatFitsAtTheTop(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	app.Do(ActionResize{Height: 100})

	app.Do(ActionScrollHalfDown{})

	if got := app.Frame().Scroll; got != 0 {
		t.Errorf("Scroll = %d on a document that fits, want 0", got)
	}

	if got := cursorOf(app); got == "" {
		t.Error("the cursor did not move")
	}
}

// The terminal has not said how big it is yet, and there may be no document at
// all. Neither is a reason to fail.
func TestScrollHalfDoesNothingWithoutAWindow(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	app.Do(ActionScrollHalfDown{})

	if got := app.Frame().Scroll; got != 0 {
		t.Errorf("Scroll = %d with no window, want 0", got)
	}

	if got := cursorOf(app); got != "/name" {
		t.Errorf("scrolling with no window selected %q, want one node down", got)
	}

	empty := New(Deps{JSONView: documentview.NewJSONRenderer(), TreeView: documentview.NewTreeRenderer()})
	empty.Do(ActionScrollHalfDown{})
	empty.Do(ActionScrollHalfUp{})

	if frame := empty.Frame(); frame.Cursor != -1 || frame.Scroll != 0 {
		t.Errorf("Frame() = %+v with no document open", frame)
	}
}
