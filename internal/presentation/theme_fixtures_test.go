package presentation

import (
	"strings"
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
