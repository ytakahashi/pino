package application

import (
	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// Frame is one drawable picture of the session: the rows of the document,
// which of them the cursor is on, and where the window onto them starts.
//
// The three arrive together because drawing needs all three and rendering
// produces them at once. Separate accessors would each have to render the
// document again to answer, which is the same trap StatusInfo avoids by not
// carrying a row count.
//
// The rows are the whole document rather than the part that fits: how many
// rows there are belongs in the status bar, and cutting to the width of the
// terminal is the business of whoever owns it. Scroll says where that cut
// begins.
type Frame struct {
	Lines []documentview.Line

	// Cursor indexes Lines, and is -1 when no row is selected: nothing is
	// open, or the document is empty.
	Cursor int

	// Scroll is the first row to draw.
	Scroll int

	// Matches indexes the rows that contain a match or hide one in a folded
	// subtree. It is sorted and contains no duplicates.
	Matches []int
}

// matchingRows maps node matches onto the rows that represent them in this
// frame. Closing rows share a path with their opening row and cannot take the
// cursor, so they must not replace the opening row in the lookup.
func matchingRows(lines []documentview.Line, paths []domain.Path) []int {
	if len(paths) == 0 {
		return nil
	}

	rowByPointer := make(map[string]int, len(lines))
	for row, line := range lines {
		if line.Kind != documentview.LineClose {
			rowByPointer[line.Path.String()] = row
		}
	}

	marked := make(map[int]struct{}, len(paths))
	for _, match := range paths {
		for visible := match; ; visible = visible.Parent() {
			if row, ok := rowByPointer[visible.String()]; ok {
				marked[row] = struct{}{}

				break
			}

			if visible.IsRoot() {
				break
			}
		}
	}

	var rows []int
	for row := range lines {
		if _, ok := marked[row]; ok {
			rows = append(rows, row)
		}
	}

	return rows
}
