// Package textwrap cuts a line of text into rows that fit a terminal.
//
// It measures characters the way the presentation layer draws them, so a row
// it produces is never clipped when drawn. The application borrows that
// measurement through a port rather than importing a terminal library itself.
package textwrap

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Wrapper wraps a logical line by display width.
type Wrapper struct{}

func New() *Wrapper { return &Wrapper{} }

// WrapLine cuts one logical line into rows of at most width columns.
//
// The line must not contain control characters below U+0020, as the port
// requires: they are measured as no columns, so neither the cut nor a check of
// the result would notice one overrunning the screen.
//
// Spaces at a cut are kept rather than dropped, so that joining the rows gives
// the line back. The inspector wraps with the same function, which is what
// keeps the two measuring alike.
func (*Wrapper) WrapLine(text string, width int) []string {
	// A wide character takes two columns. Below that, Hardwrap would either
	// leave text whole (width < 1) or open with an empty row and still overrun
	// (width 1), so both are answered with the text whole.
	if width < 2 {
		return []string{text}
	}

	return strings.Split(ansi.Hardwrap(text, width, true), "\n")
}
