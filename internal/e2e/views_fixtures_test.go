package e2e

import (
	"slices"
	"strings"
	"testing"
)

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
