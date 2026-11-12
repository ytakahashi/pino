package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/cli"
)

func TestTheProgramOpensAFileChosenFromADirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, data := range map[string]string{"b.json": `{"host":"chosen-host"}`, "a.json": `{"host":"other-host"}`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tm, _, screen := startDocument(t, cli.DirectoryDocument{Dir: dir}, cli.ProgramConfig{}, "b.json")
	if !strings.Contains(screenRow(screen, 0), "a.json") || !strings.Contains(screenRow(screen, 1), "b.json") || !strings.Contains(statusRow(screen), "2/2") {
		t.Errorf("picker screen =\n%s\nwant sorted candidates and their count", strings.Join(screen, "\n"))
	}

	tm.Type("j")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	screen = finalScreen(t, tm)
	if !strings.Contains(strings.Join(screen, "\n"), `"host": "chosen-host"`) || !strings.Contains(statusRow(screen), "b.json") {
		t.Errorf("screen =\n%s\nwant the selected document and its filename", strings.Join(screen, "\n"))
	}
}

func TestTheProgramKeepsPickingAfterASyntaxError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{\n  \"host\" \"localhost\"\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The default 80 columns are kept: the reason must fit beside the name
	// even though the temporary directory's path is long.
	tm, waiter, _ := startDocument(t, cli.DirectoryDocument{Dir: dir}, cli.ProgramConfig{}, "bad.json")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	screen := waiter.wait(t, func(screen []string) bool {
		return strings.Contains(strings.Join(screen, "\n"), "bad.json: 2:10: invalid character")
	})
	if !strings.Contains(screenRow(screen, 0), "bad.json") || !strings.Contains(statusRow(screen), "1/1") {
		t.Errorf("screen =\n%s\nwant the picker to remain open", strings.Join(screen, "\n"))
	}
	finalScreen(t, tm)
}
