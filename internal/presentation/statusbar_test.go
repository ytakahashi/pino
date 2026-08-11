package presentation

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
)

// statusText is the bar without styling, with the run of spaces between its
// two ends collapsed to the separator that divides the fields. What is left is
// the fields in the order they are drawn.
func statusText(theme Theme, info application.StatusInfo, lines int, pending Pending, width int) string {
	bar := ansi.Strip(theme.RenderStatusBar(info, lines, pending, width))

	return strings.TrimRight(gapPattern.ReplaceAllString(bar, separator), " ")
}

// gapPattern matches the space holding the two ends of the bar apart.
var gapPattern = regexp.MustCompile(` {3,}`)

func TestRenderStatusBarFields(t *testing.T) {
	open := application.StatusInfo{
		Mode:     application.ModeNormal,
		ViewMode: application.ViewJSON,
		Name:     "config.json",
		Indent:   "  ",
	}

	tests := []struct {
		name  string
		info  application.StatusInfo
		lines int
		want  string
	}{
		{
			name:  "open document",
			info:  open,
			lines: 11,
			want:  " NORMAL  JSON  config.json  11 lines  indent:2",
		},
		{
			name:  "single line",
			info:  open,
			lines: 1,
			want:  " NORMAL  JSON  config.json  1 line  indent:2",
		},
		{
			// A document nothing has been read into still has a mode and a
			// view, and the bar says so rather than going blank.
			name:  "nothing open",
			info:  application.StatusInfo{Indent: "  "},
			lines: 0,
			want:  " NORMAL  JSON  0 lines  indent:2",
		},
		{
			name:  "unsaved changes",
			info:  withDirty(open),
			lines: 11,
			want:  " NORMAL  JSON  config.json  11 lines  indent:2  modified",
		},
		{
			name:  "tabs",
			info:  withIndent(open, "\t"),
			lines: 11,
			want:  " NORMAL  JSON  config.json  11 lines  indent:tab",
		},
		{
			name:  "four spaces",
			info:  withIndent(open, "    "),
			lines: 11,
			want:  " NORMAL  JSON  config.json  11 lines  indent:4",
		},
		{
			// Only an explicit choice produces this: a document detected as
			// having no indentation falls back to the default instead.
			name:  "no indentation",
			info:  withIndent(open, ""),
			lines: 11,
			want:  " NORMAL  JSON  config.json  11 lines  indent:none",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusText(Theme{}, tc.info, tc.lines, PendingNone, 80); got != tc.want {
				t.Errorf("RenderStatusBar() = %q, want %q", got, tc.want)
			}
		})
	}
}

// Where the selection is, and what it is, sit next to the name of the file:
// the bar reads left to right as which mode, which file, which node.
func TestRenderStatusBarShowsTheSelection(t *testing.T) {
	open := application.StatusInfo{
		Mode:     application.ModeNormal,
		ViewMode: application.ViewJSON,
		Name:     "config.json",
		Indent:   "  ",
	}

	tests := []struct {
		name string
		info application.StatusInfo
		want string
	}{
		{
			name: "a node",
			info: withCursor(open, "/server/port", "number"),
			want: " NORMAL  JSON  config.json  /server/port  number  11 lines  indent:2",
		},
		{
			// RFC 6901 writes the root as nothing at all, which on a bar reads
			// as nothing being selected.
			name: "the root",
			info: withCursor(open, "", "object"),
			want: " NORMAL  JSON  config.json  /  object  11 lines  indent:2",
		},
		{
			// The type is what says something is selected, since the root's
			// pointer is empty too.
			name: "nothing selected",
			info: open,
			want: " NORMAL  JSON  config.json  11 lines  indent:2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusText(Theme{}, tc.info, 11, PendingNone, 80); got != tc.want {
				t.Errorf("RenderStatusBar() = %q, want %q", got, tc.want)
			}
		})
	}
}

// A prefix key that has been typed is shown at the far end, so that pressing
// one and seeing nothing happen is not the same as pressing a key that does
// nothing.
func TestRenderStatusBarShowsAPendingPrefix(t *testing.T) {
	info := application.StatusInfo{
		Mode:     application.ModeNormal,
		ViewMode: application.ViewJSON,
		Indent:   "  ",
	}

	tests := map[Pending]string{
		PendingNone: " NORMAL  JSON  11 lines  indent:2",
		PendingG:    " NORMAL  JSON  11 lines  indent:2  g",
		PendingZ:    " NORMAL  JSON  11 lines  indent:2  z",
	}

	for pending, want := range tests {
		t.Run(want, func(t *testing.T) {
			if got := statusText(Theme{}, info, 11, pending, 80); got != want {
				t.Errorf("RenderStatusBar() = %q, want %q", got, want)
			}
		})
	}
}

// The two ends are drawn at the two edges of the screen.
func TestRenderStatusBarKeepsTheEndsApart(t *testing.T) {
	const width = 80

	info := withDirty(withCursor(application.StatusInfo{
		Mode:     application.ModeNormal,
		ViewMode: application.ViewJSON,
		Name:     "config.json",
		Indent:   "  ",
	}, "/server/port", "number"))

	bar := ansi.Strip(Theme{}.RenderStatusBar(info, 11, PendingNone, width))

	if !strings.HasPrefix(bar, " NORMAL  JSON  config.json") {
		t.Errorf("the bar begins %q, want the mode and the file", bar)
	}

	if !strings.HasSuffix(bar, "11 lines  indent:2  modified ") {
		t.Errorf("the bar ends %q, want the state of the document", bar)
	}

	if got := lipgloss.Width(bar); got != width {
		t.Errorf("width = %d, want %d", got, width)
	}
}

// A pointer deep enough to fill the row is cut before the right hand end is:
// what is unsaved must not be the thing that disappears, and it would be
// exactly where the pointer is longest that it did.
func TestRenderStatusBarCutsTheLeftEndFirst(t *testing.T) {
	info := withDirty(withCursor(application.StatusInfo{
		Mode:     application.ModeNormal,
		ViewMode: application.ViewJSON,
		Name:     "config.json",
		Indent:   "  ",
	}, "/a/deeply/nested/pointer/that/goes/on/and/on/and/on", "string"))

	for _, width := range []int{80, 60, 45, 40} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			bar := ansi.Strip(Theme{}.RenderStatusBar(info, 11, PendingNone, width))

			if !strings.HasSuffix(bar, "modified ") {
				t.Errorf("the bar ends %q, want it still to say the document is modified", bar)
			}

			if got := lipgloss.Width(bar); got != width {
				t.Errorf("width = %d, want %d", got, width)
			}
		})
	}
}

// A screen too narrow to hold the right hand end at all gives it up rather
// than drawing the two on top of one another.
func TestRenderStatusBarDropsTheRightEndOnANarrowScreen(t *testing.T) {
	info := withDirty(application.StatusInfo{
		Mode:     application.ModeNormal,
		ViewMode: application.ViewJSON,
		Name:     "config.json",
		Indent:   "  ",
	})

	for _, width := range []int{28, 20, 8, 1} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			bar := ansi.Strip(Theme{}.RenderStatusBar(info, 11, PendingNone, width))

			if got := lipgloss.Width(bar); got != width {
				t.Errorf("width = %d, want %d", got, width)
			}

			// What is left is the beginning of the left hand end, cut like
			// any other row rather than run together with the right.
			if !strings.HasPrefix(" NORMAL  JSON  config.json", strings.TrimRight(bar, " ")) {
				t.Errorf("the bar reads %q, want the beginning of the left hand end", bar)
			}
		})
	}
}

// The pointer is the second piece of text on the bar that comes from outside
// pino: its tokens are the document's own keys, and Path.String escapes only
// "~" and "/" in them. A key may hold anything a JSON string may.
func TestRenderStatusBarNeutralisesThePointer(t *testing.T) {
	tests := []struct {
		name    string
		pointer string
		want    string
	}{
		{name: "newline", pointer: "/two\nlines", want: "/two�lines"},
		{name: "escape sequence", pointer: "/\x1b[31mred", want: "/�[31mred"},
		{name: "tab", pointer: "/a\tb", want: "/a�b"},
		{name: "left alone", pointer: "/設定", want: "/設定"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := application.StatusInfo{Indent: "  ", Pointer: tc.pointer, Type: "string"}

			got := Theme{}.RenderStatusBar(info, 11, PendingNone, 80)

			if strings.ContainsRune(got, '\n') {
				t.Errorf("RenderStatusBar() spans several rows: %q", got)
			}

			if strings.ContainsRune(got, '\x1b') {
				t.Errorf("RenderStatusBar() passed an escape through: %q", got)
			}

			if !strings.Contains(got, tc.want) {
				t.Errorf("RenderStatusBar() = %q, want it to hold %q", got, tc.want)
			}
		})
	}
}

// The bar occupies its width exactly, in one row. Anything wider would wrap
// and push the document off the top of the screen; anything narrower would
// leave the strip unfinished.
func TestRenderStatusBarOccupiesExactlyOneRowOfTheWidth(t *testing.T) {
	info := application.StatusInfo{
		Mode:     application.ModeNormal,
		ViewMode: application.ViewJSON,
		Name:     "a-rather-long-file-name.json",
		Indent:   "  ",
	}

	for _, width := range []int{120, 45, 12, 1} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			got := DefaultTheme().RenderStatusBar(info, 11, PendingNone, width)

			if strings.Contains(got, "\n") {
				t.Fatalf("RenderStatusBar() spans several rows: %q", got)
			}

			if w := lipgloss.Width(got); w != width {
				t.Errorf("RenderStatusBar() width = %d, want %d", w, width)
			}
		})
	}
}

// A file name is the one piece of text on the bar that comes from outside
// pino, and on Unix it may hold anything but a slash and a NUL. Neither a
// newline nor an escape sequence in one may reach the terminal: the first
// would break the bar across rows, the second would be carried out.
func TestRenderStatusBarNeutralisesTheFileName(t *testing.T) {
	tests := []struct {
		name string
		file string
		want string
	}{
		{name: "newline", file: "two\nlines.json", want: "two�lines.json"},
		{name: "carriage return", file: "back\rup.json", want: "back�up.json"},
		{name: "tab", file: "a\tb.json", want: "a�b.json"},
		{name: "delete", file: "a\x7fb.json", want: "a�b.json"},
		{name: "escape sequence", file: "\x1b[31mred\x1b[m.json", want: "�[31mred�[m.json"},
		{name: "invalid utf-8", file: "a\xffb.json", want: "a�b.json"},

		// Nothing is done to a name that has nothing wrong with it, wide
		// characters and all.
		{name: "left alone", file: "設定.json", want: "設定.json"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := application.StatusInfo{Name: tc.file, Indent: "  "}

			// The zero theme adds no escapes of its own, so what is left in
			// the result came from the name.
			got := Theme{}.RenderStatusBar(info, 11, PendingNone, 80)

			if strings.ContainsRune(got, '\n') {
				t.Errorf("RenderStatusBar() spans several rows: %q", got)
			}

			if strings.ContainsRune(got, '\x1b') {
				t.Errorf("RenderStatusBar() passed an escape through: %q", got)
			}

			if !strings.Contains(got, tc.want) {
				t.Errorf("RenderStatusBar() = %q, want it to hold %q", got, tc.want)
			}
		})
	}
}

func withDirty(info application.StatusInfo) application.StatusInfo {
	info.Dirty = true

	return info
}

func withIndent(info application.StatusInfo, indent string) application.StatusInfo {
	info.Indent = indent

	return info
}

func withCursor(info application.StatusInfo, pointer, typ string) application.StatusInfo {
	info.Pointer, info.Type = pointer, typ

	return info
}

// The tree view leaves the pointer and the type off the bar, because the
// inspector says both a row below and says them at more length. What the bar
// reports about the document itself does not change with the view.
func TestRenderStatusBarDropsTheSelectionInTheTreeView(t *testing.T) {
	open := application.StatusInfo{
		Mode:   application.ModeNormal,
		Name:   "config.json",
		Indent: "  ",
	}

	tests := map[string]struct {
		info application.StatusInfo
		want string
	}{
		"a node in the tree view": {
			info: withView(withCursor(open, "/server/port", "number"), application.ViewTree),
			want: " NORMAL  TREE  config.json  6 lines  indent:2",
		},

		// The same session drawn the other way keeps them, which is the whole
		// of the difference.
		"a node in the JSON view": {
			info: withView(withCursor(open, "/server/port", "number"), application.ViewJSON),
			want: " NORMAL  JSON  config.json  /server/port  number  6 lines  indent:2",
		},

		"the root in the tree view": {
			info: withView(withCursor(open, "", "object"), application.ViewTree),
			want: " NORMAL  TREE  config.json  6 lines  indent:2",
		},

		"nothing selected in the tree view": {
			info: withView(open, application.ViewTree),
			want: " NORMAL  TREE  config.json  6 lines  indent:2",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := statusText(Theme{}, tc.info, 6, PendingNone, 80); got != tc.want {
				t.Errorf("RenderStatusBar() = %q, want %q", got, tc.want)
			}
		})
	}
}

// The right hand end says what state the document is in, which is a fact about
// the document rather than about how it is being drawn.
func TestRenderStatusBarKeepsTheRightEndAcrossViews(t *testing.T) {
	open := application.StatusInfo{
		Mode:   application.ModeNormal,
		Name:   "config.json",
		Indent: "\t",
		Dirty:  true,
	}

	want := "6 lines  indent:tab  modified  z"

	for _, view := range []application.ViewMode{application.ViewJSON, application.ViewTree} {
		t.Run(view.String(), func(t *testing.T) {
			got := statusText(Theme{}, withView(withCursor(open, "/a", "null"), view), 6, PendingZ, 80)

			if !strings.HasSuffix(got, want) {
				t.Errorf("RenderStatusBar() = %q, want it to end with %q", got, want)
			}
		})
	}
}

func withView(info application.StatusInfo, view application.ViewMode) application.StatusInfo {
	info.ViewMode = view

	return info
}
