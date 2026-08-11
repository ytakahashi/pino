package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// Moving the selection from one node to the next: through the rows, in and
// out of containers, and to the ends of the document.

func TestMoveNextAndPrev(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	// Down through the whole document, closing rows never selected.
	want := []string{
		"/name", "/server", "/server/host", "/server/ports",
		"/server/ports/0", "/server/ports/1", "/server/tls", "/debug",
	}

	if got := cursorOf(app); got != "" {
		t.Fatalf("a freshly opened document starts at %q, want the root", got)
	}

	for i, pointer := range want {
		app.Do(ActionMoveNext{})

		if got := cursorOf(app); got != pointer {
			t.Fatalf("step %d down selected %q, want %q", i+1, got, pointer)
		}
	}

	// The last node is the end: pressing on stays put rather than landing on a
	// closing row.
	app.Do(ActionMoveNext{})

	if got := cursorOf(app); got != "/debug" {
		t.Errorf("moving past the last node selected %q, want /debug", got)
	}

	// And back up the same way.
	for i := len(want) - 2; i >= 0; i-- {
		app.Do(ActionMovePrev{})

		if got := cursorOf(app); got != want[i] {
			t.Fatalf("step %d up selected %q, want %q", len(want)-i, got, want[i])
		}
	}

	app.Do(ActionMovePrev{})

	if got := cursorOf(app); got != "" {
		t.Errorf("moving up from the first member selected %q, want the root", got)
	}

	app.Do(ActionMovePrev{})

	if got := cursorOf(app); got != "" {
		t.Errorf("moving above the root selected %q, want the root", got)
	}
}

func TestMoveInAndOut(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	// In from the root reaches the first member.
	app.Do(ActionMoveIn{})

	if got := cursorOf(app); got != "/name" {
		t.Fatalf("moving in from the root selected %q, want /name", got)
	}

	// A value has nothing to step into.
	app.Do(ActionMoveIn{})

	if got := cursorOf(app); got != "/name" {
		t.Errorf("moving in from a string selected %q, want /name", got)
	}

	// Down to a container, then in twice.
	press(app, ActionMoveNext{}, ActionMoveIn{})

	if got := cursorOf(app); got != "/server/host" {
		t.Fatalf("moving in from /server selected %q, want /server/host", got)
	}

	press(app, ActionMoveNext{}, ActionMoveIn{})

	if got := cursorOf(app); got != "/server/ports/0" {
		t.Fatalf("moving in from /server/ports selected %q, want /server/ports/0", got)
	}

	// Out walks back up, since none of these are open containers.
	app.Do(ActionMoveOut{})

	if got := cursorOf(app); got != "/server/ports" {
		t.Fatalf("moving out from an element selected %q, want /server/ports", got)
	}
}

// Moving out of an open container folds it rather than leaving it, which is
// what lets one key both close and climb.
func TestMoveOutFoldsAnOpenContainer(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	press(app, ActionMoveNext{}, ActionMoveNext{}) // /server

	if got := cursorOf(app); got != "/server" {
		t.Fatalf("expected to be on /server, got %q", got)
	}

	app.Do(ActionMoveOut{})

	if got := cursorOf(app); got != "/server" {
		t.Errorf("folding moved the cursor to %q, want it to stay on /server", got)
	}

	if !app.view.IsCollapsed(path(domain.KeySegment("server"))) {
		t.Fatal("moving out of an open container did not fold it")
	}

	// What it held is gone from the picture.
	frame := app.Frame()
	for _, l := range frame.Lines {
		if l.Path.String() == "/server/host" {
			t.Error("a folded container still shows what is inside it")
		}
	}

	// Moving on skips the whole subtree.
	app.Do(ActionMoveNext{})

	if got := cursorOf(app); got != "/debug" {
		t.Errorf("moving past a folded container selected %q, want /debug", got)
	}

	// Moving out again climbs, now that the container is closed.
	press(app, ActionMovePrev{}, ActionMoveOut{})

	if got := cursorOf(app); got != "" {
		t.Errorf("moving out of a folded container selected %q, want the root", got)
	}
}

func TestMoveInUnfolds(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	press(app, ActionMoveNext{}, ActionMoveNext{}, ActionMoveOut{}) // fold /server

	if !app.view.IsCollapsed(path(domain.KeySegment("server"))) {
		t.Fatal("the fixture did not fold")
	}

	// Unfolding leaves the cursor where it is, as vim does.
	app.Do(ActionMoveIn{})

	if app.view.IsCollapsed(path(domain.KeySegment("server"))) {
		t.Error("moving in did not unfold the container")
	}

	if got := cursorOf(app); got != "/server" {
		t.Errorf("unfolding moved the cursor to %q, want it to stay on /server", got)
	}

	// The next press steps inside.
	app.Do(ActionMoveIn{})

	if got := cursorOf(app); got != "/server/host" {
		t.Errorf("moving in after unfolding selected %q, want /server/host", got)
	}
}

// The root is an open container, but moving out of it does nothing: it is
// where walking out of a document ends, and folding it would leave a screen
// with no document on it.
func TestMoveOutOfTheRootDoesNothing(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	before := len(app.Frame().Lines)

	for range 3 {
		app.Do(ActionMoveOut{})

		if app.view.IsCollapsed(domain.Path{}) {
			t.Fatal("moving out of the root folded the whole document")
		}

		if got := cursorOf(app); got != "" {
			t.Fatalf("moving out of the root selected %q, want the root", got)
		}

		if got := len(app.Frame().Lines); got != before {
			t.Fatalf("the document draws %d rows after moving out of the root, want %d", got, before)
		}
	}
}

// Walking out of a deep selection folds each level on the way and then stops
// at the root, rather than ending by folding everything.
func TestMoveOutWalksOutAndStops(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	press(app, ActionMoveIn{}, ActionMoveNext{}, ActionMoveIn{}) // /server/host

	if got := cursorOf(app); got != "/server/host" {
		t.Fatalf("expected to be on /server/host, got %q", got)
	}

	// Out to /server, which is open, so the next press folds it, and the one
	// after that climbs to the root.
	press(app, ActionMoveOut{}, ActionMoveOut{}, ActionMoveOut{})

	if got := cursorOf(app); got != "" {
		t.Fatalf("walking out ended on %q, want the root", got)
	}

	if !app.view.IsCollapsed(path(domain.KeySegment("server"))) {
		t.Error("walking out did not fold the container it passed through")
	}

	// Pressing on at the root leaves the document as it is.
	app.Do(ActionMoveOut{})

	if app.view.IsCollapsed(domain.Path{}) {
		t.Error("pressing on at the root folded the whole document")
	}
}

// An empty container is drawn on one row and holds nothing, so neither
// direction has anywhere to go.
func TestMoveInAndOutOfAnEmptyContainer(t *testing.T) {
	t.Parallel()

	app := session(t, object(t,
		member("opts", object(t)),
		member("tags", domain.NewArray(nil)),
	))

	press(app, ActionMoveIn{}) // /opts

	if got := cursorOf(app); got != "/opts" {
		t.Fatalf("expected to be on /opts, got %q", got)
	}

	app.Do(ActionMoveIn{})

	if got := cursorOf(app); got != "/opts" {
		t.Errorf("moving into an empty object selected %q, want /opts", got)
	}

	if app.view.IsCollapsed(path(domain.KeySegment("opts"))) {
		t.Error("an empty object was folded")
	}

	// Out of it climbs, rather than folding what has nothing to hide.
	app.Do(ActionMoveOut{})

	if got := cursorOf(app); got != "" {
		t.Errorf("moving out of an empty object selected %q, want the root", got)
	}
}

func TestResizeFollowsTheCursor(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	// Three rows of room, and the cursor at the top: nothing to scroll.
	app.Do(ActionResize{Height: 3})

	if got := app.Frame().Scroll; got != 0 {
		t.Errorf("Scroll = %d on a fresh document, want 0", got)
	}

	// Walking down past the bottom of the window drags it along.
	press(app, ActionMoveNext{}, ActionMoveNext{}, ActionMoveNext{}) // /server/host, row 3

	frame := app.Frame()
	if frame.Cursor < frame.Scroll || frame.Cursor >= frame.Scroll+3 {
		t.Errorf("cursor row %d is outside the window [%d, %d)",
			frame.Cursor, frame.Scroll, frame.Scroll+3)
	}

	// Growing the terminal until the document fits puts it back at the top.
	app.Do(ActionResize{Height: 100})

	if got := app.Frame().Scroll; got != 0 {
		t.Errorf("Scroll = %d once the document fits, want 0", got)
	}

	// A window of no rows scrolls nowhere rather than doing arithmetic on it.
	app.Do(ActionResize{Height: -5})

	if got := app.Frame().Scroll; got != 0 {
		t.Errorf("Scroll = %d with no window, want 0", got)
	}
}

// Folding an ancestor of the cursor leaves it pointing at a node no longer
// drawn. The next action has to bring it back to something on screen and to
// write that back, or the status bar would go on naming the hidden node.
func TestCursorRecoversFromBeingFoldedAway(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	// Deep inside, then fold a container above without moving.
	press(app, ActionMoveIn{}, ActionMoveNext{}, ActionMoveIn{}, ActionMoveNext{}, ActionMoveIn{})

	if got := cursorOf(app); got != "/server/ports/0" {
		t.Fatalf("expected to be on /server/ports/0, got %q", got)
	}

	app.view.Collapse(path(domain.KeySegment("server")))

	app.Do(ActionResize{Height: 10})

	if got := cursorOf(app); got != "/server" {
		t.Errorf("the cursor settled on %q, want the nearest ancestor still drawn", got)
	}

	if got := app.Frame().Cursor; got < 0 {
		t.Errorf("Frame().Cursor = %d, want a row; the cursor was not written back", got)
	}
}

func TestMoveFirstAndLast(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	app.Do(ActionMoveLast{})

	if got := cursorOf(app); got != "/debug" {
		t.Errorf("moving to the last node selected %q, want /debug", got)
	}

	app.Do(ActionMoveFirst{})

	if got := cursorOf(app); got != "" {
		t.Errorf("moving to the first node selected %q, want the root", got)
	}

	// From deep inside, both still reach the ends of the document.
	press(app, ActionMoveIn{}, ActionMoveNext{}, ActionMoveIn{})

	app.Do(ActionMoveLast{})

	if got := cursorOf(app); got != "/debug" {
		t.Errorf("moving to the last node from inside selected %q, want /debug", got)
	}

	app.Do(ActionMoveFirst{})

	if got := cursorOf(app); got != "" {
		t.Errorf("moving to the first node from inside selected %q, want the root", got)
	}
}

// These two ask for an end of the document rather than for a step, so they
// work even when the cursor has been left pointing at something folded away.
func TestMoveFirstRecoversALostCursor(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	press(app, ActionMoveIn{}, ActionMoveNext{}, ActionMoveIn{}) // /server/host
	app.view.Collapse(path(domain.KeySegment("server")))

	app.Do(ActionMoveFirst{})

	if got := cursorOf(app); got != "" {
		t.Errorf("moving to the first node selected %q, want the root", got)
	}
}

// A document of one node has the same row for both ends.
func TestMoveFirstAndLastOnASingleValue(t *testing.T) {
	t.Parallel()

	app := session(t, text(t, "only"))

	app.Do(ActionMoveLast{})

	if got := cursorOf(app); got != "" {
		t.Errorf("moving to the last node selected %q, want the root", got)
	}

	if got := app.Frame().Cursor; got != 0 {
		t.Errorf("Frame().Cursor = %d, want 0", got)
	}
}
