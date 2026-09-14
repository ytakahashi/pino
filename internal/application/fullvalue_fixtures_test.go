package application

import (
	"strconv"
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// longValue is a string of length runes, long enough to be shortened. Its text
// names n, so that a block found beneath the wrong node reads as another
// node's value rather than passing for the right one.
func longValue(n, length int) string {
	var b strings.Builder
	for b.Len() < length {
		b.WriteString("v" + strconv.Itoa(n) + " abcdefgh ")
	}

	return b.String()[:length]
}

// fullValueDocument holds a value of every kind, a shortened string nested in
// a container, and one string too long for any screen a test uses.
func fullValueDocument(t *testing.T) domain.Node {
	t.Helper()

	return object(t,
		member("long", text(t, longValue(1, 100))),
		member("short", text(t, "short")),
		member("number", domain.NewNumber("12345")),
		member("bool", domain.NewBool(true)),
		member("null", domain.NewNull()),
		member("object", object(t, member("inner", text(t, longValue(2, 100))))),
		member("huge", text(t, longValue(3, 400))),
	)
}

// showingInFull opens root in view with a body of width columns and height
// rows, showing selected values in full.
func showingInFull(t *testing.T, root domain.Node, view ViewMode, width, height int) *App {
	t.Helper()

	app := sessionIn(t, root, view)
	app.Do(ActionResize{Width: width, Height: height})

	app.view.FullValue = true
	app.settle(app.render())

	return app
}

// blockOf is the run of wrap rows directly beneath the selected row of frame.
func blockOf(frame Frame) []documentview.Line {
	if frame.Cursor < 0 {
		return nil
	}

	return frame.Lines[frame.Cursor+1 : blockEnd(frame.Lines, frame.Cursor)+1]
}

// wrapRowCount is how many rows of frame are wrap rows, wherever they are.
func wrapRowCount(frame Frame) int {
	count := 0

	for _, l := range frame.Lines {
		if l.Kind == documentview.LineWrap {
			count++
		}
	}

	return count
}

// blockText is the text of a block, its rows joined back together.
func blockText(block []documentview.Line) string {
	var b strings.Builder
	for _, l := range block {
		b.WriteString(l.Text())
	}

	return b.String()
}

// shortened reports whether the row of n shows it cut short.
func shortened(a *App, n domain.Node) bool {
	if n.Kind() != domain.KindString {
		return false
	}

	return documentview.ScalarSpan(n, 0).Text != documentview.ScalarSpan(n, a.view.MaxStrLen).Text
}

// recordingTextWrap delegates wrapping and records the row limits the
// application places on it. It is a spy only for the port argument; generated
// rows remain the fake adapter's responsibility.
type recordingTextWrap struct {
	maxRows []int
}

func (w *recordingTextWrap) WrapLine(text string, width, maxRows int) ([]string, bool) {
	w.maxRows = append(w.maxRows, maxRows)

	return fakeTextWrap{}.WrapLine(text, width, maxRows)
}
