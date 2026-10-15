package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/cli"
)

func TestTheProgramSavesAnEditedStdinDocumentToTheEnteredPath(t *testing.T) {
	t.Parallel()

	path := missingPath(t)
	tm, waiter := startDocument(t, cli.StdinDocument{Data: []byte(jsoncConfig)}, cli.ProgramConfig{}, "Address used by clients")

	first := waiter.wait(t, func(screen []string) bool {
		return strings.Contains(statusRow(screen), "stdin")
	})
	if bar := statusRow(first); strings.Contains(bar, "new") || strings.Contains(bar, "modified") {
		t.Errorf("initial bar = %q, want an unchanged stdin document", bar)
	}

	tm.Type("jj")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	tm.Type("1")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(statusRow(screen), "modified")
	})

	tm.Send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(strings.Join(screen, "\n"), "Save as")
	})
	tm.Type(path)
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waiter.wait(t, func(screen []string) bool {
		bar := statusRow(screen)
		return strings.Contains(bar, filepath.Base(path)) && !strings.Contains(bar, "modified")
	})

	if got := readFile(t, path); got != savedJSONCConfig {
		t.Errorf("saved stdin document holds:\n%s\nwant:\n%s", got, savedJSONCConfig)
	}

	finalScreen(t, tm)
}
