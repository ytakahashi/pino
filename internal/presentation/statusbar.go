package presentation

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
)

// separator divides one field of the bar from the next, and is also the least
// space left between the two ends of it.
const separator = "  "

// RenderStatusBar draws the strip along the bottom of the screen.
//
// The number of lines and the prefix waiting are parameters rather than fields
// of info, because neither is the session's to report: the lines are already
// in the hands of whoever draws, and asking the application to count them
// would render the whole document a second time on every redraw, while a
// half-typed sequence never reaches the application at all.
func (t Theme) RenderStatusBar(info application.StatusInfo, lines int, pending Pending, width int) string {
	left := " " + strings.Join(leftFields(info), separator)
	right := strings.Join(rightFields(info, lines, pending), separator) + " "

	// Cutting the text before it is styled, rather than bounding the style
	// with a maximum width: a width set on a style wraps what does not fit,
	// which would turn the bar into several rows and push the document off
	// the top of the screen.
	return t.StatusBar.Width(width).Render(spread(left, right, width))
}

// leftFields say where the reader is: in which mode, in which view, in which
// file, and — where nothing else is saying so — at which node, of which type.
func leftFields(info application.StatusInfo) []string {
	fields := []string{info.Mode.String(), info.ViewMode.String()}

	// There is no document before one is opened, and the bar is still drawn:
	// the mode and the view are true of the session either way.
	if info.Name != "" {
		fields = append(fields, printable(info.Name))
	}

	switch info.ViewMode {
	case application.ViewJSON:
		// The type is what says whether anything is selected. The pointer
		// cannot: the root's is empty, and so is the one reported with nothing
		// open.
		if info.Type != "" {
			fields = append(fields, printable(pointerLabel(info.Pointer)), info.Type)
		}

	case application.ViewTree:
		// Left off, because the inspector says both a row below and says them
		// at more length. It is drawn whenever the tree is: the one size that
		// leaves no room for it leaves none for this bar either.
	}

	// Last, and on this side, because this is the end that is cut when the bar
	// does not fit. A runtime summary can be any length, and it is already
	// written out in full above: the notice holding it is what makes sure it
	// was read, so what is left here is a reminder rather than the only telling.
	if info.Notice != nil {
		fields = append(fields, printable(info.Notice.Summary))
	}

	return fields
}

// rightFields say what state the document is in, and what pino is waiting for.
func rightFields(info application.StatusInfo, lines int, pending Pending) []string {
	fields := []string{lineCount(lines), "indent:" + indentLabel(info.Indent)}

	// Two independent things, so two fields rather than one that has to choose
	// between them: a document whose file has still to be created and which
	// has been typed into is both, and the first save clears both at once.
	if info.New {
		fields = append(fields, "new")
	}

	if info.Dirty {
		fields = append(fields, "modified")
	}

	// A prefix key sits at the far end, as it does in vim: pressing one and
	// seeing nothing happen is otherwise indistinguishable from a key that
	// does nothing.
	if label := pending.String(); label != "" {
		fields = append(fields, label)
	}

	return fields
}

// spread lays the two ends of the bar out across width columns.
//
// What is unsaved has to survive a deep pointer pushing the row wide, so the
// right hand end is kept and the left is cut to fit around it. Only a screen
// too narrow to hold the right hand end at all gives it up, and then what is
// left is cut like any other row.
func spread(left, right string, width int) string {
	room := width - ansi.StringWidth(right) - len(separator)
	if room <= 0 {
		return ansi.Truncate(left, width, "")
	}

	left = ansi.Truncate(left, room, "")
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)

	return left + strings.Repeat(" ", gap) + right
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
