package application

import (
	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// This file is the arithmetic of moving around a rendered document. Every
// function here reads nothing but the rows it is given and returns a row
// number, so the rules of moving can be checked without a session, a terminal
// or a document.
//
// The cursor itself is a domain.Path, not a row number: a row number means
// something only for the renderer and the folded set that produced it, and
// would be stale the moment a view is switched, a node folded or an edit made.
// Row numbers are derived here, every time they are needed.
//
// A row number of -1 means "no such row" throughout.

// indexOf is the row the cursor sits on, or -1 when the document does not show
// it. Paths are compared by their tokens, so a cursor built from a pointer
// finds the row a renderer built from a tree.
func indexOf(lines []documentview.Line, cursor domain.Path) int {
	for i, l := range lines {
		if l.Path.Equal(cursor) {
			return i
		}
	}

	return -1
}

// visibleRow is the row for the cursor, or for the nearest ancestor of it that
// is on screen.
//
// Folding a node away leaves the cursor pointing inside it, which is what
// happens when a whole document is folded at once. Climbing to what is still
// drawn keeps the selection where the reader was looking, in the same way vim
// leaves the cursor on a fold it has just closed. Deleting a subtree will do
// the same later.
//
// The walk terminates because the root is its own parent and is always drawn.
func visibleRow(lines []documentview.Line, cursor domain.Path) int {
	for p := cursor; ; p = p.Parent() {
		if i := indexOf(lines, p); i >= 0 {
			return i
		}

		if p.IsRoot() {
			return -1
		}
	}
}

// firstRow is the row of the whole document, which is the root and always
// takes the cursor.
func firstRow(lines []documentview.Line) int {
	if len(lines) == 0 {
		return -1
	}

	return 0
}

// nextRow is the row below from that the cursor can land on, or -1 at the end
// of the document.
//
// Closing rows are stepped over: a "}" is not a node, and stopping on one
// would make moving down through a nested document pause on punctuation. This
// is what turns a flat list of rows back into a walk over the tree.
func nextRow(lines []documentview.Line, from int) int {
	for i := from + 1; i < len(lines); i++ {
		if lines[i].Kind != documentview.LineClose {
			return i
		}
	}

	return -1
}

// prevRow is the row above from that the cursor can land on, or -1 at the top.
func prevRow(lines []documentview.Line, from int) int {
	for i := from - 1; i >= 0; i-- {
		if lines[i].Kind != documentview.LineClose {
			return i
		}
	}

	return -1
}

// parentRow is the row of the container holding the node at from, or -1 for
// the root.
//
// It is the first row above with a smaller depth. A sibling container's own
// rows, its closing one included, sit at the same depth as the node itself, so
// none of them can be mistaken for the parent.
//
// The parent of a drawn row is always drawn, which is why this can work from
// rows alone. Climbing by path, as visibleRow does, is for a cursor that may
// have no row at all.
func parentRow(lines []documentview.Line, from int) int {
	if from < 0 || from >= len(lines) {
		return -1
	}

	for i := from - 1; i >= 0; i-- {
		if lines[i].Depth < lines[from].Depth {
			return i
		}
	}

	return -1
}

// firstChildRow is the row of the first child of the node at from, or -1 when
// it has none on screen.
//
// Only an open container has children drawn below it, and its first child
// begins on the very next row. A folded container answers -1 here: it is
// unfolded first, and moving into it is a second keystroke.
func firstChildRow(lines []documentview.Line, from int) int {
	if from < 0 || from >= len(lines) || lines[from].Kind != documentview.LineOpen {
		return -1
	}

	if from+1 >= len(lines) {
		return -1
	}

	return from + 1
}

// lastRow is the final row the cursor can land on, or -1 when there is none.
//
// It is not the final row: a document ends in the closing rows of everything
// still open, and the end of a document means its last node.
func lastRow(lines []documentview.Line) int {
	return nearestRow(lines, len(lines)-1, -1)
}

// nearestRow is the closest row to from that the cursor can land on, searching
// in direction dir first (positive downwards) and turning around if that end
// offers nothing.
//
// Jumping by a count of rows can land on a closing row, and the run of them
// that ends a document has nothing below it, which is why the search has to be
// able to turn around. from outside the document is treated as its nearest
// end.
func nearestRow(lines []documentview.Line, from, dir int) int {
	if len(lines) == 0 {
		return -1
	}

	from = min(max(from, 0), len(lines)-1)

	step := 1
	if dir < 0 {
		step = -1
	}

	if row := scanRow(lines, from, step); row >= 0 {
		return row
	}

	return scanRow(lines, from, -step)
}

// scanRow walks from towards one end of the document, answering the first row
// the cursor can land on.
func scanRow(lines []documentview.Line, from, step int) int {
	for i := from; i >= 0 && i < len(lines); i += step {
		if lines[i].Kind != documentview.LineClose {
			return i
		}
	}

	return -1
}

// intoWindow is the row to select when from has been left outside the window
// of height rows starting at scroll, or -1 when it is still inside it.
//
// Moving the window on its own can strand the selection above or below what is
// drawn. Bringing it to the edge it went out by is the smallest way back, and
// keeps reading on with the wheel from carrying the selection along further
// than the text moved.
func intoWindow(lines []documentview.Line, from, scroll, height int) int {
	switch {
	case from < scroll:
		return nearestRow(lines, scroll, 1)

	case from >= scroll+height:
		return nearestRow(lines, scroll+height-1, -1)

	default:
		return -1
	}
}

// clampScroll is the first row to draw so that cursor is on screen, moving as
// little as possible from scroll.
//
// Scrolling follows the cursor rather than leading it, and no margin is kept
// above or below, which is what vim does unless told otherwise.
func clampScroll(scroll, cursor, height, total int) int {
	// Before the terminal has said how big it is there is no window to fit
	// anything into, so nothing is scrolled. Drawing starts once a size
	// arrives, and this begins working then rather than having left a
	// meaningless offset behind.
	if height <= 0 {
		return 0
	}

	// A document that fits is shown from the top, whatever was scrolled to
	// before it shrank.
	if total <= height {
		return 0
	}

	if cursor >= 0 {
		if cursor < scroll {
			scroll = cursor
		}

		if cursor >= scroll+height {
			scroll = cursor - height + 1
		}
	}

	// Folding rows away can leave the window past the end of a document that
	// is still longer than the screen.
	return min(max(scroll, 0), total-height)
}
