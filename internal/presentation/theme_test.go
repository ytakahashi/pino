package presentation

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application/documentview"
)

// The layout tests use the zero Theme, whose styles emit nothing, so that
// what is asserted is the text and not the escape sequences around it.

func TestRenderLineIndentsByDepth(t *testing.T) {
	tests := []struct {
		name   string
		indent string
		depth  int
		want   string
	}{
		{name: "root", indent: "  ", depth: 0, want: "null"},
		{name: "one level", indent: "  ", depth: 1, want: "  null"},
		{name: "three levels", indent: "  ", depth: 3, want: "      null"},
		{name: "tabs", indent: "\t", depth: 2, want: "\t\tnull"},
		{name: "four spaces", indent: "    ", depth: 2, want: "        null"},

		// A document written without indentation draws flat. The width comes
		// from the file rather than from the theme, so this is the same
		// answer the status bar gives.
		{name: "no indent", indent: "", depth: 3, want: "null"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line := documentview.Line{
				Depth: tc.depth,
				Spans: []documentview.Span{{Text: "null", Role: documentview.RoleNullValue}},
			}

			if got := (Theme{}).RenderLine(line, tc.indent, rowMarks{}); got != tc.want {
				t.Errorf("RenderLine() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderLineWritesSpansInOrder(t *testing.T) {
	line := documentview.Line{
		Depth: 1,
		Spans: []documentview.Span{
			{Text: `"host"`, Role: documentview.RoleKey},
			{Text: ": ", Role: documentview.RolePunct},
			{Text: `"localhost"`, Role: documentview.RoleStringValue},
			{Text: ",", Role: documentview.RolePunct},
		},
	}

	want := `  "host": "localhost",`

	if got := (Theme{}).RenderLine(line, "  ", rowMarks{}); got != want {
		t.Errorf("RenderLine() = %q, want %q", got, want)
	}
}

func TestRenderLineHandlesEmptySpans(t *testing.T) {
	if got := (Theme{}).RenderLine(documentview.Line{}, "  ", rowMarks{}); got != "" {
		t.Errorf("RenderLine() = %q, want %q", got, "")
	}
}

// TestDefaultThemeStylesEveryRoleDistinctly is what answers the question the
// Role model was written to raise: whether the role alone carries enough for
// the display to colour a document. It fails both when a role is drawn
// unstyled and when two of them are indistinguishable on screen.
func TestDefaultThemeStylesEveryRoleDistinctly(t *testing.T) {
	theme := DefaultTheme()
	seen := make(map[string]documentview.Role, len(allRoles))

	for _, role := range allRoles {
		line := documentview.Line{Spans: []documentview.Span{{Text: "x", Role: role}}}
		got := theme.RenderLine(line, "", rowMarks{})

		if got == "x" {
			t.Errorf("role %v renders unstyled", role)

			continue
		}

		if other, ok := seen[got]; ok {
			t.Errorf("roles %v and %v render alike as %q", other, role, got)
		}

		seen[got] = role
	}
}

// The band behind the selected row has to be unbroken, which holds only while
// every span leaves its background for the cursor to set. A Role that painted
// its own would keep it, and the row would come out striped.
func TestDefaultThemeLeavesTheBackgroundToTheCursor(t *testing.T) {
	theme := DefaultTheme()
	unset := lipgloss.NewStyle().GetBackground()

	for _, role := range allRoles {
		if bg := theme.style(role).GetBackground(); bg != unset {
			t.Errorf("role %v sets a background (%v); the cursor could not paint over it", role, bg)
		}
	}
}

// Every part of the selected row is drawn in the cursor's styling: the spans,
// which keep their own colours besides, and the indentation in front of them.
func TestRenderLineMarksTheSelectedRow(t *testing.T) {
	theme := DefaultTheme()

	line := documentview.Line{
		Depth: 1,
		Spans: []documentview.Span{
			{Text: `"host"`, Role: documentview.RoleKey},
			{Text: ": ", Role: documentview.RolePunct},
			{Text: `"localhost"`, Role: documentview.RoleStringValue},
		},
	}

	plain := theme.RenderLine(line, "  ", rowMarks{})
	selected := theme.RenderLine(line, "  ", rowMarks{Selected: true})

	if selected == plain {
		t.Fatal("the selected row is drawn exactly like an unselected one")
	}

	// The text is untouched; only what surrounds it changes.
	if got, want := ansi.Strip(selected), ansi.Strip(plain); got != want {
		t.Errorf("the selected row reads %q, want %q", got, want)
	}

	marker := cursorBackground(t, theme)

	// The band opens before the indentation rather than after it, so that it
	// starts at the left edge of the row.
	if !strings.HasPrefix(selected, "\x1b["+marker+"m") {
		t.Errorf("the selected row is %q, want the cursor's background first", selected)
	}

	// And every span carries it, or the band would break at the first colour.
	// The parameter is counted rather than the whole escape sequence, since a
	// span with a colour of its own has both set in one.
	if got, want := strings.Count(selected, marker), len(line.Spans)+1; got != want {
		t.Errorf("the cursor's background appears %d times, want %d (one per span, plus the indent)", got, want)
	}

	if strings.Contains(plain, marker) {
		t.Error("a row that is not selected carries the cursor's background")
	}
}

func TestRenderCursorFillUsesTheCursorBackground(t *testing.T) {
	theme := DefaultTheme()

	if got := theme.RenderCursorFill(0); got != "" {
		t.Errorf("RenderCursorFill(0) = %q, want empty", got)
	}

	if got := theme.RenderCursorFill(-3); got != "" {
		t.Errorf("RenderCursorFill(-3) = %q, want empty", got)
	}

	got := theme.RenderCursorFill(4)

	if want := "    "; ansi.Strip(got) != want {
		t.Errorf("RenderCursorFill(4) = %q, want %d columns of space", ansi.Strip(got), len(want))
	}

	if !strings.Contains(got, cursorBackground(t, theme)) {
		t.Errorf("RenderCursorFill(4) = %q, want the cursor's background", got)
	}
}

// A match is marked independently from selection, so moving the cursor onto
// the row does not make the search result disappear.
func TestRenderLineMarksAMatchUnderTheCursor(t *testing.T) {
	theme := DefaultTheme()
	line := documentview.Line{
		Depth: 1,
		Spans: []documentview.Span{
			{Text: `"host"`, Role: documentview.RoleKey},
			{Text: ": ", Role: documentview.RolePunct},
			{Text: `"localhost"`, Role: documentview.RoleStringValue},
		},
	}

	plain := theme.RenderLine(line, "  ", rowMarks{})
	matched := theme.RenderLine(line, "  ", rowMarks{Matched: true})
	selected := theme.RenderLine(line, "  ", rowMarks{Selected: true, Matched: true})

	if got, want := ansi.Strip(matched), ansi.Strip(plain); got != want {
		t.Errorf("the matched row reads %q, want %q", got, want)
	}

	if !strings.Contains(matched, "\x1b[4m") {
		t.Error("the rendered match carries no underline")
	}

	if !theme.decorate(lipgloss.NewStyle(), rowMarks{Matched: true}).GetUnderline() {
		t.Error("the matched row is not underlined")
	}

	decorated := theme.decorate(lipgloss.NewStyle(), rowMarks{Selected: true, Matched: true})
	if !decorated.GetUnderline() {
		t.Error("the selected match lost its underline")
	}

	if got := decorated.GetBackground(); got != theme.Cursor.GetBackground() {
		t.Errorf("the selected match background = %v, want the cursor's %v",
			got, theme.Cursor.GetBackground())
	}

	if strings.Contains(plain, "\x1b[4m") {
		t.Error("an unmatched row carries an underline")
	}

	if got, want := ansi.Strip(selected), ansi.Strip(plain); got != want {
		t.Errorf("the selected match reads %q, want %q", got, want)
	}
}

// A role beyond the ones the theme knows loses its colour, not its text: the
// document stays readable while the gap in the theme is being noticed.
func TestRenderLineDrawsUnknownRoleUnstyled(t *testing.T) {
	unknown := documentview.Role(len(allRoles))

	line := documentview.Line{Spans: []documentview.Span{{Text: "x", Role: unknown}}}

	if got := DefaultTheme().RenderLine(line, "", rowMarks{}); got != "x" {
		t.Errorf("RenderLine() = %q, want %q", got, "x")
	}
}

func TestRenderTooSmallUsesTheErrorStyle(t *testing.T) {
	tests := map[string]struct {
		width, height int
		want          []string
	}{
		// The size in the mock of the design: what is needed, and what there is.
		"the whole message": {
			width: 34, height: 6,
			want: []string{"terminal too small", "needs 60x12, has 34x6", "", "", "", ""},
		},

		// Narrower than the message, which is cut like any other row rather
		// than wrapped onto a screen that has no rows to spare.
		"cut to the width": {
			width: 10, height: 3,
			want: []string{"terminal t", "needs 60x1", ""},
		},

		// A screen of one row keeps the reason and gives up the numbers.
		"one row": {width: 40, height: 1, want: []string{"terminal too small"}},

		// Nothing to draw in is nothing drawn, rather than arithmetic on a
		// negative height.
		"no rows":        {width: 40, height: 0, want: []string{""}},
		"nothing at all": {width: 0, height: 0, want: []string{""}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := strings.Split(DefaultTheme().RenderTooSmall(tc.width, tc.height), "\n")

			if len(got) != len(tc.want) {
				t.Fatalf("RenderTooSmall(%d, %d) drew %d rows, want %d",
					tc.width, tc.height, len(got), len(tc.want))
			}

			for i, w := range tc.want {
				if got[i] != w {
					t.Errorf("row %d = %q, want %q", i, got[i], w)
				}
			}
		})
	}
}
