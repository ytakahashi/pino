package presentation

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
)

// allRoles is every role a renderer can put on a span. A role added without a
// case in Theme.style would draw unstyled, which the styling test below
// catches even though the map is not a switch.
var allRoles = []application.Role{
	application.RoleKey,
	application.RoleStringValue,
	application.RoleNumberValue,
	application.RoleBoolValue,
	application.RoleNullValue,
	application.RolePunct,
	application.RoleTreeGuide,
}

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
			line := application.Line{
				Depth: tc.depth,
				Spans: []application.Span{{Text: "null", Role: application.RoleNullValue}},
			}

			if got := (Theme{}).RenderLine(line, tc.indent, false); got != tc.want {
				t.Errorf("RenderLine() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderLineWritesSpansInOrder(t *testing.T) {
	line := application.Line{
		Depth: 1,
		Spans: []application.Span{
			{Text: `"host"`, Role: application.RoleKey},
			{Text: ": ", Role: application.RolePunct},
			{Text: `"localhost"`, Role: application.RoleStringValue},
			{Text: ",", Role: application.RolePunct},
		},
	}

	want := `  "host": "localhost",`

	if got := (Theme{}).RenderLine(line, "  ", false); got != want {
		t.Errorf("RenderLine() = %q, want %q", got, want)
	}
}

func TestRenderLineHandlesEmptySpans(t *testing.T) {
	if got := (Theme{}).RenderLine(application.Line{}, "  ", false); got != "" {
		t.Errorf("RenderLine() = %q, want %q", got, "")
	}
}

// TestDefaultThemeStylesEveryRoleDistinctly is what answers the question the
// Role model was written to raise: whether the role alone carries enough for
// the display to colour a document. It fails both when a role is drawn
// unstyled and when two of them are indistinguishable on screen.
func TestDefaultThemeStylesEveryRoleDistinctly(t *testing.T) {
	theme := DefaultTheme()
	seen := make(map[string]application.Role, len(allRoles))

	for _, role := range allRoles {
		line := application.Line{Spans: []application.Span{{Text: "x", Role: role}}}
		got := theme.RenderLine(line, "", false)

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

	line := application.Line{
		Depth: 1,
		Spans: []application.Span{
			{Text: `"host"`, Role: application.RoleKey},
			{Text: ": ", Role: application.RolePunct},
			{Text: `"localhost"`, Role: application.RoleStringValue},
		},
	}

	plain := theme.RenderLine(line, "  ", false)
	selected := theme.RenderLine(line, "  ", true)

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

// cursorBackground is the escape sequence parameter the cursor's styling sets.
// It is what to look for in a row, since the sequence around it differs with
// whatever colour the span has of its own.
func cursorBackground(t *testing.T, theme Theme) string {
	t.Helper()

	const esc = "\x1b["

	rendered := theme.Cursor.Render("x")

	start := strings.Index(rendered, esc)
	end := strings.IndexByte(rendered, 'm')

	if start < 0 || end <= start {
		t.Fatalf("the cursor style renders %q, with no styling to look for", rendered)
	}

	return rendered[start+len(esc) : end]
}

func TestRenderCursorFill(t *testing.T) {
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

// A role beyond the ones the theme knows loses its colour, not its text: the
// document stays readable while the gap in the theme is being noticed.
func TestRenderLineDrawsUnknownRoleUnstyled(t *testing.T) {
	unknown := application.Role(len(allRoles))

	line := application.Line{Spans: []application.Span{{Text: "x", Role: unknown}}}

	if got := DefaultTheme().RenderLine(line, "", false); got != "x" {
		t.Errorf("RenderLine() = %q, want %q", got, "x")
	}
}
