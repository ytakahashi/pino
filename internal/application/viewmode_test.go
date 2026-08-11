package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// Drawing the same document the other way, and keeping the reader where they
// were as it is redrawn.

func TestViewModeCyclesThroughEveryView(t *testing.T) {
	t.Parallel()

	if got := ViewJSON.Next(); got != ViewTree {
		t.Errorf("ViewJSON.Next() = %v, want %v", got, ViewTree)
	}

	if got := ViewTree.Next(); got != ViewJSON {
		t.Errorf("ViewTree.Next() = %v, want %v; two views make a round trip", got, ViewJSON)
	}
}

// Which renderer draws is decided by the view and by nothing else, so a
// session holding two of them uses one at a time.
func TestRendererFollowsTheView(t *testing.T) {
	t.Parallel()

	jsonView := &spyRenderer{lines: []Line{{Kind: LineSingle}}}
	treeView := &spyRenderer{lines: []Line{
		{Kind: LineOpen},
		{Path: path(domain.KeySegment("a")), Kind: LineSingle, Depth: 1},
	}}

	app := New(Deps{
		Parser:   &fakeParser{root: testTree(t)},
		Files:    fakeFileStore{data: map[string][]byte{"a.json": []byte(testSource)}},
		JSONView: jsonView,
		TreeView: treeView,
	})

	if err := app.Open("a.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	if got := len(app.Frame().Lines); got != 1 {
		t.Errorf("the JSON view drew %d rows, want the 1 its renderer returns", got)
	}

	if treeView.calls != 0 {
		t.Errorf("the tree renderer was called %d times while the JSON view was showing", treeView.calls)
	}

	app.Do(ActionToggleView{})

	if got := app.ViewMode(); got != ViewTree {
		t.Fatalf("ViewMode() = %v after Tab, want %v", got, ViewTree)
	}

	if got := len(app.Frame().Lines); got != 2 {
		t.Errorf("the tree view drew %d rows, want the 2 its renderer returns", got)
	}

	if treeView.calls == 0 {
		t.Error("the tree renderer was never called while the tree view was showing")
	}
}

// The selection is held as a path and both views draw a row for it, so Tab has
// nothing to do to keep it. The folded set is shared for the same reason, and
// switching must not fold or unfold anything.
func TestToggleViewKeepsTheSelection(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	app.Do(ActionResize{Height: 20})

	// Fold something, then select a node outside it.
	press(app, ActionMoveNext{}, ActionMoveNext{}, ActionMoveNext{}, ActionMoveNext{}) // /server/ports
	app.Do(ActionMoveOut{})                                                            // folds it, cursor stays

	press(app, ActionMovePrev{}) // /server/host

	if got := cursorOf(app); got != "/server/host" {
		t.Fatalf("the cursor is at %q before switching, want /server/host", got)
	}

	folded := describe(app.view.Collapsed)

	app.Do(ActionToggleView{})

	if got := app.ViewMode(); got != ViewTree {
		t.Errorf("ViewMode() = %v after Tab, want %v", got, ViewTree)
	}

	if got := cursorOf(app); got != "/server/host" {
		t.Errorf("the cursor moved to %q, want /server/host", got)
	}

	if got := describe(app.view.Collapsed); got != folded {
		t.Errorf("the folded set became %s, want %s; the two views share one set", got, folded)
	}
}

// Where the cursor sits within the window is what survives a switch, not the
// offset of the window itself: the row a node sits on is not the same in the
// two views, so an offset carried over unchanged would show somewhere else.
func TestToggleViewKeepsTheCursorInTheWindow(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	app.Do(ActionResize{Height: 5})

	// /server/tls, which the JSON view draws below a closing row and the tree
	// view does not: the two put it on different rows, which is the whole
	// difficulty.
	for range 7 {
		app.Do(ActionMoveNext{})
	}

	if got := cursorOf(app); got != "/server/tls" {
		t.Fatalf("the cursor is at %q, want /server/tls", got)
	}

	before := app.Frame()

	app.Do(ActionToggleView{})

	after := app.Frame()

	if before.Cursor-before.Scroll != after.Cursor-after.Scroll {
		t.Errorf("the cursor moved from row %d of the window to row %d",
			before.Cursor-before.Scroll, after.Cursor-after.Scroll)
	}

	// The window did have to move, or the check above would hold for a Tab
	// that left everything alone.
	if before.Scroll == after.Scroll {
		t.Errorf("Scroll stayed at %d across views drawing the node on rows %d and %d",
			before.Scroll, before.Cursor, after.Cursor)
	}
}

// Tab is its own opposite: pressing it twice is being back on the same node,
// and away from the ends of a document, looking at it from the same place.
func TestToggleViewReturnsToTheSameNode(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	app.Do(ActionResize{Height: 5})

	for range 7 {
		app.Do(ActionMoveNext{})
	}

	want := app.Frame()
	cursor := cursorOf(app)

	press(app, ActionToggleView{}, ActionToggleView{})

	if got := app.ViewMode(); got != ViewJSON {
		t.Errorf("ViewMode() = %v after two presses, want %v", got, ViewJSON)
	}

	if got := cursorOf(app); got != cursor {
		t.Errorf("the cursor came back to %q, want %q", got, cursor)
	}

	if got := app.Frame(); got.Scroll != want.Scroll {
		t.Errorf("Scroll came back to %d, want %d", got.Scroll, want.Scroll)
	}
}

// Near the end of a document the window cannot always go where the switch
// would put it: the view with fewer rows cannot draw a node close to its end
// as far down the screen. settle brings the offset back into range, and the
// next switch reads the corrected one.
//
// What is promised there is the node, not the row of the window it sits on.
// This fixes how much is given up: the selection is exactly where it was, the
// cursor is on screen throughout, and the shift the ends impose happens once
// instead of growing with every press.
func TestToggleViewAtTheEndOfADocument(t *testing.T) {
	t.Parallel()

	const height = 5

	app := session(t, sample(t))
	app.Do(ActionResize{Height: height})
	app.Do(ActionScrollBy{Rows: 100}) // to the bottom of the JSON view

	cursor := cursorOf(app)
	if cursor != "/server/tls" {
		t.Fatalf("the bottom of the JSON view selected %q, want /server/tls", cursor)
	}

	// Five presses: tree, JSON, tree, JSON, tree.
	frames := make([]Frame, 0, 5)

	for i := range 5 {
		app.Do(ActionToggleView{})

		if got := cursorOf(app); got != cursor {
			t.Fatalf("press %d selected %q, want %q", i+1, got, cursor)
		}

		f := app.Frame()
		if f.Cursor < f.Scroll || f.Cursor >= f.Scroll+height {
			t.Errorf("press %d left the cursor at row %d, outside the window [%d, %d)",
				i+1, f.Cursor, f.Scroll, f.Scroll+height)
		}

		frames = append(frames, f)
	}

	// Each view settles on one window and stays there, rather than the two
	// pushing each other a little further on every press.
	if frames[0].Scroll != frames[2].Scroll || frames[2].Scroll != frames[4].Scroll {
		t.Errorf("the tree view showed rows from %d, %d and %d; the shift is accumulating",
			frames[0].Scroll, frames[2].Scroll, frames[4].Scroll)
	}

	if frames[1].Scroll != frames[3].Scroll {
		t.Errorf("the JSON view showed rows from %d and then %d; the shift is accumulating",
			frames[1].Scroll, frames[3].Scroll)
	}
}

// A folded document is where the two views have least in common on screen and
// most in common underneath: the same nodes are hidden, by the same set.
func TestToggleViewWithDeepFolding(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	app.Do(ActionResize{Height: 20})

	// Walk in to an element and back out, folding each container on the way.
	for range 5 {
		app.Do(ActionMoveNext{})
	}

	press(app, ActionMoveOut{}, ActionMoveOut{}, ActionMoveOut{}, ActionMoveOut{})

	if got := cursorOf(app); got != "/server" {
		t.Fatalf("the cursor is at %q after folding outwards, want /server", got)
	}

	folded := describe(app.view.Collapsed)

	app.Do(ActionToggleView{})

	if got := cursorOf(app); got != "/server" {
		t.Errorf("the cursor moved to %q, want /server", got)
	}

	if got := describe(app.view.Collapsed); got != folded {
		t.Errorf("the folded set became %s, want %s", got, folded)
	}

	// The row it landed on is folded in this view too, or h and l would mean
	// different things either side of the switch.
	frame := app.Frame()
	if frame.Cursor < 0 || !frame.Lines[frame.Cursor].Collapsed {
		t.Errorf("the row at %d is not folded in the tree view: %+v", frame.Cursor, frame.Lines)
	}
}

func TestToggleViewWithoutDocument(t *testing.T) {
	t.Parallel()

	app := New(Deps{JSONView: NewJSONRenderer(), TreeView: NewTreeRenderer()})

	app.Do(ActionToggleView{})

	if got := app.ViewMode(); got != ViewTree {
		t.Errorf("ViewMode() = %v, want %v; the view is a property of the session", got, ViewTree)
	}

	if frame := app.Frame(); frame.Lines != nil || frame.Cursor != -1 || frame.Scroll != 0 {
		t.Errorf("Frame() = %+v with no document open", frame)
	}
}

// Switching views changes how much room the document has, so the layer drawing
// it reports the new height straight after. Arriving second, that report must
// not undo what the switch just arranged.
func TestResizeAfterToggleViewKeepsThePosition(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	app.Do(ActionResize{Height: 5})

	for range 7 {
		app.Do(ActionMoveNext{})
	}

	app.Do(ActionToggleView{})

	want := app.Frame()
	cursor := cursorOf(app)

	app.Do(ActionResize{Height: 5})

	if got := cursorOf(app); got != cursor {
		t.Errorf("the cursor moved to %q, want %q", got, cursor)
	}

	if got := app.Frame(); got.Scroll != want.Scroll {
		t.Errorf("Scroll moved to %d, want %d; settling twice settles to the same place",
			got.Scroll, want.Scroll)
	}
}

// TestMovementAgreesAcrossViews drives the same keys through both views and
// checks that the selection and the folded set come out the same at each step.
//
// It is the counterpart of TestViewsAgreeOnRows one layer up. That fixes the
// rows the two renderers offer; this fixes what walking them arrives at, which
// is what makes cursor.go free of any knowledge of which view is showing.
//
// Half a screen and the wheel are left out on purpose. Both move by a count of
// rows, and the two views do not have the same rows, so reading on by half a
// screen reaching a different node in each is right rather than wrong. The
// window is left out for the same reason: it is measured in rows.
func TestMovementAgreesAcrossViews(t *testing.T) {
	t.Parallel()

	script := []struct {
		name string
		act  Action
	}{
		{"resize", ActionResize{Height: 5}},
		{"l", ActionMoveIn{}},
		{"l", ActionMoveIn{}},
		{"j", ActionMoveNext{}},
		{"j", ActionMoveNext{}},
		{"l", ActionMoveIn{}},
		{"j", ActionMoveNext{}},
		{"k", ActionMovePrev{}},
		{"h", ActionMoveOut{}},
		{"h", ActionMoveOut{}},
		{"G", ActionMoveLast{}},
		{"gg", ActionMoveFirst{}},
		{"zM", ActionCollapseAll{}},
		{"j", ActionMoveNext{}},
		{"l", ActionMoveIn{}},
		{"l", ActionMoveIn{}},
		{"G", ActionMoveLast{}},
		{"zR", ActionExpandAll{}},
		{"G", ActionMoveLast{}},
		{"h", ActionMoveOut{}},
		{"resize", ActionResize{Height: 2}},
		{"k", ActionMovePrev{}},
		{"gg", ActionMoveFirst{}},
	}

	for name, doc := range documents(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			asJSON := sessionIn(t, doc.root, ViewJSON)
			asTree := sessionIn(t, doc.root, ViewTree)

			for i, s := range script {
				asJSON.Do(s.act)
				asTree.Do(s.act)

				if got, want := cursorOf(asTree), cursorOf(asJSON); got != want {
					t.Fatalf("step %d (%s): the tree view selected %q, the JSON view %q",
						i, s.name, got, want)
				}

				if got, want := describe(asTree.view.Collapsed), describe(asJSON.view.Collapsed); got != want {
					t.Fatalf("step %d (%s): the tree view folded %s, the JSON view %s",
						i, s.name, got, want)
				}
			}
		})
	}
}
