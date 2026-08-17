package e2e

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Help crosses the terminal key table and application flow, but temporarily
// replaces only the presentation. Closing it therefore redraws the exact
// document screen that was present before it opened.
func TestTheProgramReturnsFromHelpToTheSameDocument(t *testing.T) {
	t.Parallel()

	tm, waiter := start(t, "localhost")

	tm.Type("jj")

	before := waiter.wait(t, func(screen []string) bool {
		return strings.Contains(statusRow(screen), "/server/cache  object")
	})

	tm.Type("?")

	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(strings.Join(screen, "\n"), "pino help") &&
			strings.Contains(statusRow(screen), "HELP")
	})

	tm.Send(tea.KeyPressMsg{Code: tea.KeyEscape})

	after := waiter.wait(t, func(screen []string) bool {
		return strings.Contains(statusRow(screen), "NORMAL") &&
			strings.Contains(statusRow(screen), "/server/cache  object")
	})

	if !slices.Equal(after, before) {
		t.Errorf("the screen after help differs from the one it replaced:\n%s", strings.Join(after, "\n"))
	}

	finalScreen(t, tm)
}

// Ctrl+c keeps the safe-quit contract while help is covering a dirty
// document. Cancelling the question returns to the document, not to help, and
// leaves the edit for the reader to save or discard.
func TestTheProgramSafelyQuitsFromHelpWithUnsavedWork(t *testing.T) {
	t.Parallel()

	tm, waiter := start(t, "localhost")

	tm.Type("jjjjj")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	tm.Type("1")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(strings.Join(screen, "\n"), `"port": 80801`) &&
			strings.Contains(statusRow(screen), "modified")
	})

	tm.Type("?")

	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(strings.Join(screen, "\n"), "pino help") &&
			strings.Contains(statusRow(screen), "modified")
	})

	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(strings.Join(screen, "\n"), "unsaved changes")
	})

	tm.Type("c")

	returned := waiter.wait(t, func(screen []string) bool {
		return strings.Contains(strings.Join(screen, "\n"), `"port": 80801`) &&
			strings.Contains(statusRow(screen), "NORMAL") &&
			strings.Contains(statusRow(screen), "modified")
	})

	if got := strings.Join(returned, "\n"); strings.Contains(got, "pino help") {
		t.Errorf("cancelling safe quit returned to help:\n%s", got)
	}

	finalScreenDiscarding(t, tm, waiter)
}
