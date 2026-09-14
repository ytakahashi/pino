// Package textwrap cuts a line of text into rows that fit a terminal.
//
// It measures characters the way the presentation layer draws them, so a row
// it produces is never clipped when drawn. The application borrows that
// measurement through a port rather than importing a terminal library itself.
package textwrap

import "github.com/charmbracelet/x/ansi"

// Wrapper wraps a logical line by display width.
type Wrapper struct{}

func New() *Wrapper { return &Wrapper{} }

// WrapLine cuts one logical line into at most maxRows rows of at most width
// columns. It reports truncation as soon as another row would be needed, so
// work and allocation are bounded by what the caller can display.
//
// The line must not contain control characters below U+0020, as the port
// requires: they are measured as no columns, so neither the cut nor a check of
// the result would notice one overrunning the screen.
//
// Spaces at a cut are kept rather than dropped. The same grapheme measurement
// as ansi.Hardwrap is used, which keeps this adapter aligned with the inspector
// and with the presentation layer that clips rows.
func (*Wrapper) WrapLine(text string, width, maxRows int) ([]string, bool) {
	if maxRows < 1 {
		return nil, text != ""
	}

	// A wide character takes two columns. In a narrower row the loop below
	// could never fit one: it would cut an empty row in front of it and still
	// overrun with it, so such a width is answered with the text whole.
	if width < 2 {
		return []string{text}, false
	}

	// A byte cannot occupy more than one row, so text length keeps an
	// accidentally enormous terminal height from becoming an enormous
	// allocation before any text has been examined.
	rows := make([]string, 0, min(maxRows, len(text)+1))
	start, at, rowWidth := 0, 0, 0

	for at < len(text) {
		cluster, clusterWidth := ansi.FirstGraphemeCluster(text[at:], ansi.GraphemeWidth)
		if rowWidth+clusterWidth > width {
			rows = append(rows, text[start:at])
			if len(rows) == maxRows {
				return rows, true
			}

			start, rowWidth = at, 0
		}

		at += len(cluster)
		rowWidth += clusterWidth
	}

	return append(rows, text[start:]), false
}
