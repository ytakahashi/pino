package e2e

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application"
	"github.com/ytakahashi/pino/internal/cli"
)

// Saving crosses every boundary pino has: a key press, a state transition, an
// encoder, and a real file replaced on a real file system. What is asserted is
// the file, since that is the only thing a save is for.
func TestTheProgramSavesAnEditedDocument(t *testing.T) {
	t.Parallel()

	tm, waiter, path := startAt(t, writeConfig(t), "localhost")

	// The port is the last node in traversal order.
	tm.Type("jjjjj")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	tm.Type("1")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(statusRow(screen), "modified")
	})

	tm.Send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

	// The bar stops saying the document is unsaved, which is what the session
	// says once the write has committed.
	saved := waiter.wait(t, func(screen []string) bool {
		return !strings.Contains(statusRow(screen), "modified")
	})

	if got := statusRow(saved); !strings.Contains(got, "config.json") {
		t.Errorf("the bar reads %q, want it still to name the file", got)
	}

	// The file holds the edit, laid out the way it was written: four spaces,
	// as the document on disk uses, and a newline at the end.
	want := strings.Replace(config, `"port": 8080`, `"port": 80801`, 1)

	if got := readFile(t, path); got != want {
		t.Errorf("the file holds:\n%s\nwant:\n%s", got, want)
	}

	// Nothing is unsaved, so leaving asks nothing.
	if screen := finalScreen(t, tm); !strings.Contains(strings.Join(screen, "\n"), "80801") {
		t.Error("the final screen does not hold the saved value")
	}
}

// Leaving with unsaved changes asks, and the answer that throws them away
// leaves the file exactly as it was.
func TestTheProgramLeavesTheFileAloneWhenChangesAreDiscarded(t *testing.T) {
	t.Parallel()

	tm, waiter, path := startAt(t, writeConfig(t), "localhost")

	tm.Type("jjjjj")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	tm.Type("1")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(statusRow(screen), "modified")
	})

	screen := finalScreenDiscarding(t, tm, waiter)

	if got := strings.Join(screen, "\n"); !strings.Contains(got, "80801") {
		t.Errorf("the final screen does not hold the edit that was discarded:\n%s", got)
	}

	if got := readFile(t, path); got != config {
		t.Errorf("the file holds:\n%s\nwant it unchanged:\n%s", got, config)
	}
}

// A path holding nothing opens as an empty document, and the file is created
// by saving rather than by opening.
func TestTheProgramCreatesAFileThatWasNotThere(t *testing.T) {
	t.Parallel()

	path := missingPath(t)

	tm, waiter, _ := startAt(t, path, "{}")

	if _, err := os.Stat(path); err == nil {
		t.Fatal("opening a path that holds nothing created the file")
	}

	first := waiter.wait(t, func(screen []string) bool {
		return strings.Contains(statusRow(screen), "new")
	})

	if got := statusRow(first); strings.Contains(got, "modified") {
		t.Errorf("the bar reads %q, want nothing unsaved in a document nobody has typed into", got)
	}

	tm.Send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

	waiter.wait(t, func(screen []string) bool {
		return !strings.Contains(statusRow(screen), "new")
	})

	if got, want := readFile(t, path), "{}\n"; got != want {
		t.Errorf("the file holds %q, want %q", got, want)
	}

	finalScreen(t, tm)
}

// The width asked for on the command line reaches the file, beating the one
// the document was read with.
func TestTheProgramSavesWithTheIndentAskedFor(t *testing.T) {
	t.Parallel()

	cfg := cli.ProgramConfig{Application: application.Config{
		IndentOverride: "  ", OverrideIndent: true,
	}}

	tm, waiter, path := startWith(t, writeConfig(t), cfg, "localhost")

	first := waiter.wait(t, func(screen []string) bool {
		return strings.Contains(statusRow(screen), "indent:2")
	})

	if got := statusRow(first); strings.Contains(got, "indent:4") {
		t.Errorf("the bar reads %q, want the width asked for rather than the file's", got)
	}

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

	// The whole file is laid out again, since the width it was read with is
	// not the width it is written with.
	want := "{\n" +
		"  \"server\": {\n" +
		"    \"cache\": {\n" +
		"      \"ttl\": 60\n" +
		"    },\n" +
		"    \"host\": \"localhost\"\n" +
		"  },\n" +
		"  \"port\": 80801\n" +
		"}\n"

	if got := readFile(t, path); got != want {
		t.Errorf("the file holds:\n%s\nwant:\n%s", got, want)
	}

	finalScreen(t, tm)
}

// A write that could not be carried out is reported in words about the file
// the reader named, and the session goes on holding the document. Losing the
// edits because the file system said no is the one outcome a save must not
// have.
//
// The failure is made by taking the directory away rather than by taking write
// permission off it. Permission bits answer differently depending on who the
// test runs as, while a path that is not there fails the same way on every
// machine, and the store reports it before anything on disk has been touched.
func TestTheProgramKeepsTheDocumentWhenTheWriteFails(t *testing.T) {
	t.Parallel()

	path := missingPath(t)

	tm, waiter, _ := startAt(t, path, "{}")

	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(statusRow(screen), "new")
	})

	// Somebody else takes the directory away while pino holds the document.
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		t.Fatalf("RemoveAll() = %v", err)
	}

	tm.Send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

	// What is on screen names the file and the operation, rather than the
	// temporary file the store got as far as trying to make.
	failed := waiter.wait(t, func(screen []string) bool {
		return strings.Contains(strings.Join(screen, "\n"), "Could not save fresh.json.")
	})

	if got := strings.Join(failed, "\n"); !strings.Contains(got, "[o] OK") {
		t.Errorf("the notice offers no way out:\n%s", got)
	}

	// Nothing was written, so the document is exactly as unsaved as it was.
	if got := statusRow(failed); !strings.Contains(got, "new") {
		t.Errorf("the bar reads %q, want the document still waiting to be created", got)
	}

	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(%s) = %v, want nothing at the path", path, err)
	}

	// Acknowledging the notice puts the reader back on the document rather
	// than ending the session with it.
	tm.Type("o")

	acknowledged := waiter.wait(t, func(screen []string) bool {
		return strings.Contains(statusRow(screen), "NORMAL")
	})

	if got := strings.Join(acknowledged, "\n"); !strings.Contains(got, "{}") {
		t.Errorf("the document is not back on screen:\n%s", got)
	}

	finalScreen(t, tm)
}

// A file that changed underneath the session is not overwritten by pressing
// the key that saves: what to do about it is put to the reader, and cancelling
// leaves both the document and the file as they are.
func TestTheProgramAsksBeforeOverwritingAChangedFile(t *testing.T) {
	t.Parallel()

	tm, waiter, path := startAt(t, writeConfig(t), "localhost")

	tm.Type("jjjjj")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	tm.Type("1")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(statusRow(screen), "modified")
	})

	// Somebody else writes the file while pino holds it.
	outside := strings.Replace(config, `"host": "localhost"`, `"host": "elsewhere"`, 1)

	if err := os.WriteFile(path, []byte(outside), 0o600); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}

	tm.Send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(strings.Join(screen, "\n"), "changed outside pino")
	})

	tm.Type("c")

	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(statusRow(screen), "NORMAL")
	})

	if got := readFile(t, path); got != outside {
		t.Errorf("the file holds:\n%s\nwant what was written outside pino:\n%s", got, outside)
	}

	// The document is still unsaved, and still the reader's to do something
	// with.
	screen := finalScreenDiscarding(t, tm, waiter)

	if got := strings.Join(screen, "\n"); !strings.Contains(got, "80801") {
		t.Errorf("the final screen does not hold the edit:\n%s", got)
	}
}
