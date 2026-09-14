package application

import (
	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// minBlockWidth is the narrowest body a value is written out in. A row cut
// short of the whole value needs room for one character, which may take two
// columns, and for the mark saying that the value goes on.
const minBlockWidth = 3

// expandSelected is lines with the selected value written out in full on the
// rows beneath its own, when the session is showing values in full and that
// value was shortened. Otherwise it is lines.
//
// The rows a renderer produced are left as they are: the row being expanded
// keeps its shortened form and the block goes after it. That keeps the width of
// the screen and the position of the cursor out of rendering, which stays a
// function of the tree, the folds and the shortening alone, and it gives both
// views the same block without either renderer knowing of it.
//
// Only a shortened string has anything to write out. Numbers, booleans and null
// are never shortened, a container is opened by unfolding it, and a string
// already shown whole would only be repeated.
//
// The block sits at no depth and takes the whole width of the body, as the
// later rows of a block comment do, so that where to cut it needs no knowledge
// of how wide indentation is drawn.
//
// A new slice is returned rather than lines grown in place, since a renderer
// may hand out rows it keeps.
func (a *App) expandSelected(lines []documentview.Line) []documentview.Line {
	if !a.view.FullValue || a.doc == nil || a.width < minBlockWidth || a.height <= 1 {
		return lines
	}

	row := indexOf(lines, a.view.Cursor)
	if row < 0 {
		return lines
	}

	n, ok := domain.Resolve(a.doc.Root(), a.view.Cursor)
	if !ok || n.Kind() != domain.KindString {
		return lines
	}

	// The value is spelled the way its row spells it, escapes included. That
	// is also what keeps control characters from the terminal and from the
	// wrapping port, which must not be given them.
	full := documentview.ScalarSpan(n, 0)
	if full.Text == documentview.ScalarSpan(n, a.view.MaxStrLen).Text {
		return lines
	}

	block := a.wrapRows(full, lines[row].Path)

	out := make([]documentview.Line, 0, len(lines)+len(block))
	out = append(out, lines[:row+1]...)
	out = append(out, block...)

	return append(out, lines[row+1:]...)
}

// blockEnd is the last row of the block written out beneath row, or row itself
// when there is none.
func blockEnd(lines []documentview.Line, row int) int {
	if row < 0 {
		return row
	}

	end := row
	for end+1 < len(lines) && lines[end+1].Kind == documentview.LineWrap {
		end++
	}

	return end
}

// wrapRows is span cut into rows as wide as the body, and at most one fewer
// of them than the body is tall.
//
// The limit is what lets the row being expanded and the whole block be on
// screen together. A value too long for that is cut, and its last row ends in
// the mark a shortened row ends in. That row is cut again one column narrower
// before the mark goes on, since a row already filling the width would push
// the mark past the edge, where drawing clips it. A value longer than a screen
// is read in the prompt that editing it opens, which scrolls.
func (a *App) wrapRows(span documentview.Span, p domain.Path) []documentview.Line {
	limit := a.height - 1
	rows, truncated := a.deps.Wrapper.WrapLine(span.Text, a.width, limit)

	if truncated {
		lastRow := len(rows) - 1
		last, _ := a.deps.Wrapper.WrapLine(rows[lastRow], a.width-1, 1)
		rows[lastRow] = last[0] + "…"
	}

	block := make([]documentview.Line, 0, len(rows))
	for _, text := range rows {
		block = append(block, documentview.Line{
			Path:  p,
			Kind:  documentview.LineWrap,
			Spans: []documentview.Span{{Text: text, Role: span.Role}},
		})
	}

	return block
}
