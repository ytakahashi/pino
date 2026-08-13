package presentation

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/domain"
)

var (
	enterKey  = special(tea.KeyEnter)
	escapeKey = special(tea.KeyEscape)
)

// band is the rows the prompt takes, as they reach the screen.
func band(t *testing.T, m Model) []string {
	t.Helper()

	l := m.layout()
	if l.PromptHeight == 0 {
		return nil
	}

	drawn := rows(t, m)

	// The band sits between the document and the status bar, which is the last
	// row of the screen.
	end := len(drawn) - statusBarRows

	return drawn[end-l.PromptHeight : end]
}

// statusRowOf is the bar along the bottom, without styling.
func statusRowOf(t *testing.T, m Model) string {
	t.Helper()

	drawn := rows(t, m)

	return drawn[len(drawn)-1]
}

func equalRows(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// awkwardDocument holds one value under "v", so that a value can be opened for
// editing by moving down one row.
func awkwardDocument(t *testing.T, value string) domain.Node {
	t.Helper()

	v, err := domain.NewString(value)
	if err != nil {
		t.Fatalf("NewString(%q) = %v", value, err)
	}

	root, err := domain.NewObject([]domain.Member{{Key: "v", Value: v}})
	if err != nil {
		t.Fatalf("NewObject() = %v", err)
	}

	return root
}
