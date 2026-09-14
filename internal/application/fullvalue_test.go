package application

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ytakahashi/pino/internal/application/documentview"
)

func TestFullValueWritesTheSelectedValueOutBeneathItsRow(t *testing.T) {
	t.Parallel()

	const width = 30

	for _, view := range []ViewMode{ViewJSON, ViewTree} {
		t.Run(view.String(), func(t *testing.T) {
			t.Parallel()

			app := showingInFull(t, fullValueDocument(t), view, width, 20)
			standOn(t, app, "/long")

			frame := app.Frame()

			block := blockOf(frame)
			if len(block) == 0 {
				t.Fatalf("nothing is written out beneath /long:\n%s", dumpLines(frame.Lines))
			}

			for i, l := range block {
				if l.Kind.Selectable() || l.Depth != 0 || !l.Path.Equal(app.view.Cursor) {
					t.Errorf("block row %d is %s at depth %d for %q, want a wrap row at depth 0 for /long",
						i, l.Kind, l.Depth, l.Path.String())
				}

				if len(l.Spans) != 1 || l.Spans[0].Role != documentview.RoleStringValue {
					t.Errorf("block row %d has spans %+v, want one string span", i, l.Spans)
				}

				if got := utf8.RuneCountInString(l.Text()); got > width {
					t.Errorf("block row %d is %d columns, want at most %d", i, got, width)
				}
			}

			if got, want := blockText(block), documentview.ScalarSpan(nodeAt(t, app, "/long"), 0).Text; got != want {
				t.Errorf("the block reads %q, want %q", got, want)
			}

			if got := wrapRowCount(frame); got != len(block) {
				t.Errorf("%d wrap rows are drawn, want only the %d beneath the selection", got, len(block))
			}
		})
	}
}

// Only a value its row shows cut short has anything more to show.
func TestFullValueOpensOnlyAShortenedString(t *testing.T) {
	t.Parallel()

	for _, ptr := range []string{"", "/short", "/number", "/bool", "/null", "/object"} {
		t.Run(ptr, func(t *testing.T) {
			t.Parallel()

			app := showingInFull(t, fullValueDocument(t), ViewJSON, 30, 20)
			standOn(t, app, ptr)

			frame := app.Frame()

			if got := wrapRowCount(frame); got != 0 {
				t.Errorf("%d wrap rows are drawn on %q, want none:\n%s", got, ptr, dumpLines(frame.Lines))
			}

			if got, want := len(frame.Lines), len(app.render()); got != want {
				t.Errorf("%d rows are drawn on %q, want the renderer's %d", got, ptr, want)
			}
		})
	}
}

// The row being expanded stays on screen with the whole block, so a value too
// long for that is cut, and says so.
func TestFullValueStopsOneRowShortOfTheBody(t *testing.T) {
	t.Parallel()

	const width, height = 20, 6

	app := showingInFull(t, fullValueDocument(t), ViewJSON, width, height)
	standOn(t, app, "/huge")

	block := blockOf(app.Frame())
	if got, want := len(block), height-1; got != want {
		t.Fatalf("the block is %d rows, want %d", got, want)
	}

	for i, l := range block {
		if got := utf8.RuneCountInString(l.Text()); got > width {
			t.Errorf("block row %d %q is %d columns, want at most %d", i, l.Text(), got, width)
		}
	}

	joined := blockText(block)

	head, marked := strings.CutSuffix(joined, "…")
	if !marked {
		t.Errorf("the block ends %q, want the mark of a cut value", joined)
	}

	if full := documentview.ScalarSpan(nodeAt(t, app, "/huge"), 0).Text; !strings.HasPrefix(full, head) {
		t.Errorf("the block reads %q, want the beginning of %q", head, full)
	}
}

// The wrapper owns the row limit so a value much longer than the screen is
// not first expanded into rows that are immediately discarded.
func TestFullValueBoundsWrappingToTheVisibleRows(t *testing.T) {
	t.Parallel()

	const width, height = 20, 6

	app := session(t, fullValueDocument(t))
	app.Do(ActionResize{Width: width, Height: height})
	standOn(t, app, "/huge")

	wrapper := &recordingTextWrap{}
	app.deps.Wrapper = wrapper
	app.view.FullValue = true
	app.Frame()

	if want := []int{height - 1, 1}; !slices.Equal(wrapper.maxRows, want) {
		t.Errorf("WrapLine row limits = %v, want %v", wrapper.maxRows, want)
	}
}

// Before a size arrives, or in a body with no room beside the row being
// expanded, nothing is written out.
func TestFullValueNeedsRoomToOpen(t *testing.T) {
	t.Parallel()

	tests := map[string]struct{ width, height int }{
		"no size":    {width: 0, height: 0},
		"too narrow": {width: minBlockWidth - 1, height: 20},
		"one row":    {width: 30, height: 1},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := showingInFull(t, fullValueDocument(t), ViewJSON, tt.width, tt.height)
			standOn(t, app, "/long")

			if got := wrapRowCount(app.Frame()); got != 0 {
				t.Errorf("%d wrap rows are drawn in %dx%d, want none", got, tt.width, tt.height)
			}
		})
	}
}

// Every action that moves the cursor or the window leaves the block beneath
// the node it ends on, and the cursor on screen. The rows an action moves over
// are taken before it moves, so this is what keeps the block from being fitted
// to the node just left.
func TestTheBlockFollowsTheCursor(t *testing.T) {
	t.Parallel()

	const width, height = 20, 8

	script := []struct {
		name string
		act  Action
	}{
		{"j", ActionMoveNext{}},
		{"j", ActionMoveNext{}},
		{"j", ActionMoveNext{}},
		{"j", ActionMoveNext{}},
		{"j", ActionMoveNext{}},
		{"j", ActionMoveNext{}},
		{"j", ActionMoveNext{}},
		{"j", ActionMoveNext{}},
		{"k", ActionMovePrev{}},
		{"k", ActionMovePrev{}},
		{"h", ActionMoveOut{}},
		{"l", ActionMoveIn{}},
		{"G", ActionMoveLast{}},
		{"k", ActionMovePrev{}},
		{"gg", ActionMoveFirst{}},
		{"ctrl+d", ActionScrollHalfDown{}},
		{"ctrl+d", ActionScrollHalfDown{}},
		{"ctrl+u", ActionScrollHalfUp{}},
		{"wheel down", ActionScrollBy{Rows: 3}},
		{"wheel up", ActionScrollBy{Rows: -2}},
		{"tab", ActionToggleView{}},
		{"j", ActionMoveNext{}},
		{"G", ActionMoveLast{}},
		{"tab", ActionToggleView{}},
		{"gg", ActionMoveFirst{}},
	}

	for _, view := range []ViewMode{ViewJSON, ViewTree} {
		t.Run(view.String(), func(t *testing.T) {
			t.Parallel()

			app := showingInFull(t, fullValueDocument(t), view, width, height)

			for i, s := range script {
				app.Do(s.act)

				frame := app.Frame()
				if frame.Cursor < 0 || !frame.Lines[frame.Cursor].Kind.Selectable() ||
					!frame.Lines[frame.Cursor].Path.Equal(app.view.Cursor) {
					t.Fatalf("step %d (%s): the cursor row %d does not hold %q:\n%s",
						i, s.name, frame.Cursor, cursorOf(app), dumpLines(frame.Lines))
				}

				if frame.Cursor < frame.Scroll || frame.Cursor >= frame.Scroll+height {
					t.Errorf("step %d (%s): the cursor row %d is outside the window [%d, %d)",
						i, s.name, frame.Cursor, frame.Scroll, frame.Scroll+height)
				}

				block := blockOf(frame)

				if end := frame.Cursor + len(block); end >= frame.Scroll+height {
					t.Errorf("step %d (%s): the block beneath %q ends on row %d, below the window [%d, %d)",
						i, s.name, cursorOf(app), end, frame.Scroll, frame.Scroll+height)
				}

				if got := wrapRowCount(frame); got != len(block) {
					t.Errorf("step %d (%s): %d wrap rows are drawn, want only the %d beneath %q:\n%s",
						i, s.name, got, len(block), cursorOf(app), dumpLines(frame.Lines))
				}

				if want := shortened(app, nodeAt(t, app, cursorOf(app))); (len(block) > 0) != want {
					t.Errorf("step %d (%s): %d rows are written out beneath %q, want a block: %t",
						i, s.name, len(block), cursorOf(app), want)
				}
			}
		})
	}
}

// A value opened on the bottom row of the window is scrolled up far enough to
// show the whole of its block, rather than the least the cursor needs.
func TestOpeningAValueAtTheBottomBringsItsBlockOnScreen(t *testing.T) {
	t.Parallel()

	const width, height = 20, 8

	app := showingInFull(t, fullValueDocument(t), ViewJSON, width, height)
	app.view.FullValue = false
	standOn(t, app, "/object/inner")

	if frame := app.Frame(); frame.Cursor != frame.Scroll+height-1 {
		t.Fatalf("the fixture puts /object/inner on row %d of the window, want the bottom one",
			frame.Cursor-frame.Scroll)
	}

	app.view.FullValue = true
	app.settle(app.render())

	frame := app.Frame()

	block := blockOf(frame)
	if len(block) == 0 {
		t.Fatal("nothing is written out beneath /object/inner")
	}

	if frame.Cursor < frame.Scroll {
		t.Errorf("the cursor row %d is above the window starting at %d", frame.Cursor, frame.Scroll)
	}

	if end := frame.Cursor + len(block); end >= frame.Scroll+height {
		t.Errorf("the block ends on row %d, below the window [%d, %d)", end, frame.Scroll, frame.Scroll+height)
	}
}

// The key starts and stops writing values out, and leaves the shortening the
// rows are drawn with to whatever chose it.
func TestTogglingFullValueTwiceLeavesTheViewAsItWas(t *testing.T) {
	t.Parallel()

	app := session(t, fullValueDocument(t))
	app.Do(ActionResize{Width: 30, Height: 20})
	standOn(t, app, "/long")

	maxStrLen := app.view.MaxStrLen

	app.Do(ActionToggleFullValue{})

	if len(blockOf(app.Frame())) == 0 || !app.Status().FullValue {
		t.Error("toggling once did not write /long out in full")
	}

	app.Do(ActionToggleFullValue{})

	if got := wrapRowCount(app.Frame()); got != 0 || app.Status().FullValue {
		t.Errorf("toggling twice left %d wrap rows and FullValue %t, want neither", got, app.Status().FullValue)
	}

	if app.view.MaxStrLen != maxStrLen {
		t.Errorf("MaxStrLen = %d after toggling, want %d", app.view.MaxStrLen, maxStrLen)
	}
}

// How one document was being looked at does not carry over to another.
func TestOpeningADocumentStopsShowingValuesInFull(t *testing.T) {
	t.Parallel()

	app := session(t, fullValueDocument(t))
	app.Do(ActionToggleFullValue{})

	if err := app.Open("a.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	if app.view.FullValue {
		t.Error("a newly opened document is shown with values in full")
	}
}

func TestTheBlockIsTheSameInBothViews(t *testing.T) {
	t.Parallel()

	asJSON := showingInFull(t, fullValueDocument(t), ViewJSON, 30, 20)
	asTree := showingInFull(t, fullValueDocument(t), ViewTree, 30, 20)

	standOn(t, asJSON, "/object/inner")
	standOn(t, asTree, "/object/inner")

	fromJSON, fromTree := blockOf(asJSON.Frame()), blockOf(asTree.Frame())

	if len(fromJSON) == 0 || len(fromJSON) != len(fromTree) || blockText(fromJSON) != blockText(fromTree) {
		t.Errorf("the JSON view writes out %q in %d rows, the tree view %q in %d",
			blockText(fromJSON), len(fromJSON), blockText(fromTree), len(fromTree))
	}
}

// A match is a node, and the block is not one: writing a value out neither
// adds matches nor moves the ones there are.
func TestFullValueLeavesSearchMatchesAlone(t *testing.T) {
	t.Parallel()

	app := showingInFull(t, fullValueDocument(t), ViewJSON, 30, 40)
	app.view.FullValue = false

	acceptSearch(t, app, "abcdefgh")
	standOn(t, app, "/long")

	before, status := matchedPointers(app.Frame()), *app.Status().Search

	app.view.FullValue = true
	app.settle(app.render())

	frame := app.Frame()
	if len(blockOf(frame)) == 0 {
		t.Fatal("nothing is written out beneath /long")
	}

	if got := matchedPointers(frame); !slices.Equal(got, before) {
		t.Errorf("matches = %q with the value written out, want %q", got, before)
	}

	if got := *app.Status().Search; got != status {
		t.Errorf("search = %+v with the value written out, want %+v", got, status)
	}
}
