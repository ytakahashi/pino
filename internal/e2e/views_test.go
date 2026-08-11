package e2e

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Tab draws the same document the other way, and the screen it lands on is one
// only a whole program produces: the tree, the inspector that comes with it,
// and a bar that has given the selection over to that inspector.
func TestSwitchesToTheTreeView(t *testing.T) {
	t.Parallel()

	tm := start(t, "localhost")

	tm.Type("j")
	tm.Type("j")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab})

	screen := finalScreen(t, tm)

	if got, want := screenRow(screen, 0), "▼ / {2}"; got != want {
		t.Errorf("row 0 = %q, want %q", got, want)
	}

	// The tree is indented at its own width whatever the file uses, so this row
	// reads the same as it would for a document indented by two.
	if got, want := screenRow(screen, 2), "    ▼ cache {1}"; got != want {
		t.Errorf("row 2 = %q, want %q", got, want)
	}

	// Where the inspector begins is the layout's decision, and the layout is
	// not this package's to name; the rule it draws says where it is.
	rule := ruleRow(t, screen)

	if got, want := screenRow(screen, rule+1), " Path      /server/cache"; got != want {
		t.Errorf("the first row of the inspector is %q, want %q", got, want)
	}

	if got := statusRow(screen); !strings.Contains(got, "NORMAL  TREE  config.json") {
		t.Errorf("the bar reads %q, want it to name the tree view", got)
	}

	if got := statusRow(screen); strings.Contains(got, "/server/cache  object") {
		t.Errorf("the bar reads %q, want the selection left to the inspector", got)
	}
}

// ruleRow is the row of the rule that divides the document from the inspector.
func ruleRow(t *testing.T, screen []string) int {
	t.Helper()

	i := slices.IndexFunc(screen, func(row string) bool {
		row = strings.TrimRight(row, " ")

		return row != "" && strings.Trim(row, "─") == ""
	})
	if i < 0 {
		t.Fatalf("no rule on the screen:\n%s", strings.Join(screen, "\n"))
	}

	return i
}
