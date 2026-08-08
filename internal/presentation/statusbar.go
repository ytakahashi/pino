package presentation

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
)

// RenderStatusBar draws the strip along the bottom of the screen.
//
// The number of lines is a parameter rather than a field of info: whoever
// draws the bar is already holding the rendered lines, and asking the
// application for a count would render the whole document a second time on
// every redraw.
func (t Theme) RenderStatusBar(info application.StatusInfo, lines, width int) string {
	fields := []string{info.Mode.String(), info.ViewMode.String()}

	// There is no document before one is opened, and the bar is still drawn:
	// the mode and the view are true of the session either way.
	if info.Name != "" {
		fields = append(fields, displayName(info.Name))
	}

	fields = append(fields, lineCount(lines), "indent:"+indentLabel(info.Indent))

	if info.Dirty {
		fields = append(fields, "modified")
	}

	// Cutting the text before it is styled, rather than bounding the style
	// with a maximum width: a width set on a style wraps what does not fit,
	// which would turn the bar into several rows and push the document off
	// the top of the screen.
	text := " " + strings.Join(fields, "  ")

	return t.StatusBar.Width(width).Render(ansi.Truncate(text, width, ""))
}

// displayName is a file name made safe to print.
//
// A name on Unix may hold any byte but a slash and a NUL, so one carrying a
// newline would break the bar across rows and one carrying an escape sequence
// would be obeyed by the terminal rather than shown. Every other piece of
// outside text is already quoted by the time it is drawn, the document's own
// strings and keys among them; the name of the file is the one that arrives
// as it was given.
//
// Each offending rune becomes a replacement character, which keeps the width
// of the name on screen the count of what it holds. Bytes that are not valid
// UTF-8 come out the same way, since decoding them yields that rune already.
func displayName(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return unicode.ReplacementChar
		}

		return r
	}, name)
}

// lineCount reads as prose rather than as a number beside a fixed noun, since
// a document of a single line is not unusual.
func lineCount(n int) string {
	if n == 1 {
		return "1 line"
	}

	return strconv.Itoa(n) + " lines"
}

// indentLabel names one level of indentation instead of showing it: a tab and
// two spaces are the same blank on screen, and which of them the file uses is
// what the reader wants to know before saving.
//
// Tabs are reported without a count because how wide one is drawn is the
// terminal's decision, so a number here would claim more than pino knows.
func indentLabel(indent string) string {
	switch {
	case indent == "":
		return "none"
	case strings.ContainsRune(indent, '\t'):
		return "tab"
	default:
		return strconv.Itoa(len(indent))
	}
}
