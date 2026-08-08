package presentation

import (
	"testing"

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

			if got := (Theme{}).RenderLine(line, tc.indent); got != tc.want {
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

	if got := (Theme{}).RenderLine(line, "  "); got != want {
		t.Errorf("RenderLine() = %q, want %q", got, want)
	}
}

func TestRenderLineHandlesEmptySpans(t *testing.T) {
	if got := (Theme{}).RenderLine(application.Line{}, "  "); got != "" {
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
		got := theme.RenderLine(line, "")

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

// A role beyond the ones the theme knows loses its colour, not its text: the
// document stays readable while the gap in the theme is being noticed.
func TestRenderLineDrawsUnknownRoleUnstyled(t *testing.T) {
	unknown := application.Role(len(allRoles))

	line := application.Line{Spans: []application.Span{{Text: "x", Role: unknown}}}

	if got := DefaultTheme().RenderLine(line, ""); got != "x" {
		t.Errorf("RenderLine() = %q, want %q", got, "x")
	}
}
