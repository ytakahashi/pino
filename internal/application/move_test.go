package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// session opens root through the real JSON renderer.
//
// Moving depends on the paths, kinds and depths of the rows agreeing with one
// another, which only a renderer produces. The fake stands in where what is
// being checked is the wiring to the ports instead.
func session(t *testing.T, root domain.Node) *App {
	t.Helper()

	app := New(Deps{
		Parser:   &fakeParser{root: root},
		Files:    fakeFiles{data: map[string][]byte{"a.json": []byte(testSource)}},
		Renderer: NewJSONRenderer(),
	})

	if err := app.Open("a.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	return app
}

// cursorOf is where the session is pointing, as a pointer.
func cursorOf(a *App) string { return a.view.Cursor.String() }

// press applies actions in order, the way a sequence of keystrokes would.
func press(a *App, actions ...Action) {
	for _, act := range actions {
		a.Do(act)
	}
}

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

func TestStatusFollowsTheCursor(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	// Every kind a value can be, reached by walking the document.
	tests := []struct {
		steps   []Action
		pointer string
		typ     string
	}{
		{steps: nil, pointer: "", typ: "object"},
		{steps: []Action{ActionMoveNext{}}, pointer: "/name", typ: "string"},
		{steps: []Action{ActionMoveNext{}}, pointer: "/server", typ: "object"},
		{steps: []Action{ActionMoveIn{}, ActionMoveNext{}}, pointer: "/server/ports", typ: "array"},
		{steps: []Action{ActionMoveIn{}}, pointer: "/server/ports/0", typ: "number"},
		{steps: []Action{ActionMoveNext{}, ActionMoveNext{}}, pointer: "/server/tls", typ: "boolean"},
	}

	for _, tt := range tests {
		press(app, tt.steps...)

		info := app.Status()

		if info.Pointer != tt.pointer {
			t.Fatalf("Status().Pointer = %q, want %q", info.Pointer, tt.pointer)
		}

		if info.Type != tt.typ {
			t.Errorf("Status().Type = %q at %q, want %q", info.Type, tt.pointer, tt.typ)
		}
	}
}

func TestStatusOfANullValue(t *testing.T) {
	t.Parallel()

	app := session(t, object(t, member("nothing", domain.NewNull())))

	app.Do(ActionMoveNext{})

	if got := app.Status().Type; got != "null" {
		t.Errorf("Status().Type = %q, want null", got)
	}
}

// Nothing is selected before a document is open, and an empty pointer alone
// cannot say so: the root has one too.
func TestStatusWithoutDocument(t *testing.T) {
	t.Parallel()

	info := New(Deps{Renderer: NewJSONRenderer()}).Status()

	if info.Pointer != "" {
		t.Errorf("Status().Pointer = %q with no document open, want empty", info.Pointer)
	}

	if info.Type != "" {
		t.Errorf("Status().Type = %q with no document open, want empty", info.Type)
	}
}

// Asking for the status must not lay the document out: the bar is refreshed on
// every frame, and drawing it a second time is what a row count would have
// cost.
func TestStatusDoesNotRender(t *testing.T) {
	t.Parallel()

	renderer := &fakeRenderer{lines: []Line{{Kind: LineOpen}}}
	app := New(Deps{
		Parser:   &fakeParser{root: sample(t)},
		Files:    fakeFiles{data: map[string][]byte{"a.json": []byte(testSource)}},
		Renderer: renderer,
	})

	if err := app.Open("a.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	before := renderer.calls
	_ = app.Status()

	if renderer.calls != before {
		t.Errorf("Status() rendered %d times, want none", renderer.calls-before)
	}
}

// Nothing here may depend on a document being open: the terminal reports its
// size before one necessarily is, and a key press is not refused either.
func TestActionsWithoutDocument(t *testing.T) {
	t.Parallel()

	actions := map[string]Action{
		"next":   ActionMoveNext{},
		"prev":   ActionMovePrev{},
		"in":     ActionMoveIn{},
		"out":    ActionMoveOut{},
		"resize": ActionResize{Height: 10},
	}

	for name, act := range actions {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := New(Deps{Renderer: NewJSONRenderer()})

			if effects := app.Do(act); effects != nil {
				t.Errorf("Do() = %v with no document open, want none", effects)
			}

			if frame := app.Frame(); frame.Cursor != -1 || frame.Scroll != 0 {
				t.Errorf("Frame() = %+v with no document open", frame)
			}
		})
	}
}
