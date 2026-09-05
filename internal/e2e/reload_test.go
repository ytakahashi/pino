package e2e

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Reloading crosses the terminal, application and file-store boundaries. A
// save after the reload also proves that the metadata used to detect outside
// changes came from the newly read file rather than the one opened at startup.
func TestTheProgramReloadsAFileChangedOutsidePino(t *testing.T) {
	t.Parallel()

	tm, waiter, path := startAt(t, writeConfig(t), "localhost")

	outside := strings.Replace(config, `"host": "localhost"`, `"host": "elsewhere"`, 1)
	if err := os.WriteFile(path, []byte(outside), 0o600); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}

	tm.Type("R")

	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(strings.Join(screen, "\n"), "elsewhere")
	})

	// The port is the last node in traversal order, so five moves reach it
	// before appending one digit.
	tm.Type("jjjjj")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	tm.Type("1")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(statusRow(screen), "modified")
	})

	tm.Send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

	waiter.wait(t, func(screen []string) bool {
		return !strings.Contains(statusRow(screen), "modified")
	})

	want := strings.Replace(outside, `"port": 8080`, `"port": 80801`, 1)
	if got := readFile(t, path); got != want {
		t.Errorf("the file holds:\n%s\nwant the outside change and the edit made after reload:\n%s", got, want)
	}

	finalScreen(t, tm)
}
