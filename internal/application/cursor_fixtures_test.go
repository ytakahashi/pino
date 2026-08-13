package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// rows renders a document the way the session does, so that the fixtures obey
// what the walking functions rely on: an open row for every close row, and a
// depth that grows by one per level. Hand-written rows can say things a
// renderer never would.
func rows(t *testing.T, root domain.Node, folded map[string]struct{}) []Line {
	t.Helper()

	return NewJSONRenderer().Render(root, RenderOptions{Collapsed: folded})
}

func pointerAt(t *testing.T, lines []Line, row int) string {
	t.Helper()

	if row < 0 || row >= len(lines) {
		t.Fatalf("row %d is out of range for %d rows", row, len(lines))
	}

	return lines[row].Path.String()
}
