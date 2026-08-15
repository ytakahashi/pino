package e2e

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Editing crosses every production boundary: terminal keys, the presentation
// model, application history, the immutable domain tree, and rendering. The
// intermediate screens make Undo and Redo part of the assertion rather than
// two actions whose final result could cancel each other out unnoticed.
func TestTheProgramEditsUndoesAndRedoesAValue(t *testing.T) {
	t.Parallel()

	tm, waiter := start(t, "localhost")

	// The port is the last node in traversal order.
	tm.Type("jjjjj")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	tm.Type("1")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	edited := waiter.wait(t, func(screen []string) bool {
		return strings.Contains(strings.Join(screen, "\n"), `"port": 80801`) &&
			strings.Contains(statusRow(screen), "modified")
	})
	if got := statusRow(edited); !strings.Contains(got, "/port  number") {
		t.Errorf("the bar reads %q, want it to name the edited number", got)
	}

	tm.Type("u")

	undone := waiter.wait(t, func(screen []string) bool {
		return strings.Contains(strings.Join(screen, "\n"), `"port": 8080`) &&
			!strings.Contains(strings.Join(screen, "\n"), `"port": 80801`) &&
			!strings.Contains(statusRow(screen), "modified")
	})
	if got := statusRow(undone); !strings.Contains(got, "/port  number") {
		t.Errorf("the bar reads %q after undo, want the cursor left at the edit", got)
	}

	tm.Send(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})

	redone := waiter.wait(t, func(screen []string) bool {
		return strings.Contains(strings.Join(screen, "\n"), `"port": 80801`) &&
			strings.Contains(statusRow(screen), "modified")
	})
	if got := statusRow(redone); !strings.Contains(got, "/port  number") {
		t.Errorf("the bar reads %q after redo, want the cursor left at the edit", got)
	}

	screen := finalScreen(t, tm)
	if got := strings.Join(screen, "\n"); !strings.Contains(got, `"port": 80801`) {
		t.Errorf("the final screen does not hold the redone value:\n%s", got)
	}
}
