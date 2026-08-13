package presentation

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/ytakahashi/pino/internal/application"
)

// The tests here drive a real Bubble Tea program writing to a pretend
// terminal, which is the only way to exercise what the pieces tested
// separately leave out: that a program built from this model starts, carries
// key presses through to the session, draws what comes back, and stops on the
// key that says so.
//
// They are layer tests and not end-to-end ones: the ports are doubled, since
// this layer is not allowed to name the adapters. What is being tested is the
// loop, not the parsing. Driving the real adapters as far as the screen is
// what internal/e2e does.
//
// Each one sends its keys and then reads the screen back from the model the
// program ended with. What reached the terminal is never searched: a terminal
// is sent only the cells that changed, so whether a piece of text arrives as
// one run depends on what was on screen before it, and the same screen can be
// written in more than one way. Searching that stream tests the diffing rather
// than pino, and answers differently on different machines.
//
// Reading only the last screen is why each test is short and stops where it
// has something to say. What the screens in between hold is settled where the
// view is drawn, against a model rather than a program.

// Moving through a document: the keys reach the key table, the session moves
// the selection, and the rows and the bar are both drawn from what comes back.
func TestTheProgramReadsADocument(t *testing.T) {
	t.Parallel()

	tm := start(t, nestedDocument(t), "localhost")

	// Down to the nested container, then into it.
	tm.Type("j")
	tm.Type("j")
	tm.Type("l")

	screen := finalScreen(t, tm)

	if got, want := screenRow(screen, 0), "{"; got != want {
		t.Errorf("row 0 = %q, want %q", got, want)
	}

	if got, want := screenRow(screen, 3), `      "ttl": 60`; got != want {
		t.Errorf("row 3 = %q, want %q", got, want)
	}

	// The bar names the node three keystrokes reached, and counts the whole
	// document rather than the part on screen.
	if got := statusRow(screen); !strings.Contains(got, "/server/cache/ttl  number") {
		t.Errorf("the bar reads %q, want it to name the selection", got)
	}

	if got := statusRow(screen); !strings.Contains(got, "9 lines  indent:2") {
		t.Errorf("the bar reads %q, want it to describe the document", got)
	}
}

// A key that means nothing on its own, followed by the one that completes it.
// Only a real program joins the prefix the table answers with, the model that
// holds it, and the session that is finally asked to act.
func TestAPrefixKeyFoldsAContainer(t *testing.T) {
	t.Parallel()

	tm := start(t, nestedDocument(t), "localhost")

	tm.Type("z")
	tm.Type("M")

	screen := finalScreen(t, tm)

	// The document folded down to its shape. The root keeps its own braces:
	// folding those would leave a screen with no document on it.
	want := []string{"{", `  "server": {…},`, `  "port": 8080`, "}", ""}

	for i, w := range want {
		if got := screenRow(screen, i); got != w {
			t.Errorf("row %d = %q, want %q", i, got, w)
		}
	}

	if got := statusRow(screen); !strings.Contains(got, "4 lines  indent:2") {
		t.Errorf("the bar reads %q, want the folded document counted", got)
	}
}

// Tab draws the same document the other way. What it lands on is a screen only
// a switch could produce: the tree, an inspector that came with it, and a bar
// that has given the selection over to that inspector.
func TestTabShowsTheTreeView(t *testing.T) {
	t.Parallel()

	tm := start(t, nestedDocument(t), "localhost")

	tm.Type("j")
	tm.Type("j")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab})

	screen := finalScreen(t, tm)

	if got, want := screenRow(screen, 0), "▼ / {2}"; got != want {
		t.Errorf("row 0 = %q, want %q", got, want)
	}

	if got, want := screenRow(screen, 2), "    ▼ cache {1}"; got != want {
		t.Errorf("row 2 = %q, want %q", got, want)
	}

	// Eighty columns puts the inspector under the tree, with a rule between.
	l := layoutFor(80, 24, application.ViewTree, 0)

	if got, want := screenRow(screen, l.BodyHeight), strings.Repeat("─", 80); got != want {
		t.Errorf("the row below the document is %q, want a rule", got)
	}

	if got, want := screenRow(screen, l.BodyHeight+1), " Path      /server/cache"; got != want {
		t.Errorf("the first row of the inspector is %q, want %q", got, want)
	}

	if got := statusRow(screen); !strings.Contains(got, "NORMAL  TREE  config.json") {
		t.Errorf("the bar reads %q, want it to name the tree view", got)
	}

	if got := statusRow(screen); strings.Contains(got, "/server/cache  object") {
		t.Errorf("the bar reads %q, want the selection left to the inspector", got)
	}
}

// The folded set belongs to the session rather than to either view: folded
// from the tree, the node comes back folded in the document as it is written.
func TestFoldStateCarriesAcrossViews(t *testing.T) {
	t.Parallel()

	tm := start(t, nestedDocument(t), "localhost")

	tm.Type("j")
	tm.Type("j")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab})
	tm.Type("h")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab})

	screen := finalScreen(t, tm)

	if got, want := screenRow(screen, 2), `    "cache": {…},`; got != want {
		t.Errorf("row 2 = %q, want %q", got, want)
	}

	if got := statusRow(screen); !strings.Contains(got, "NORMAL  JSON  config.json  /server/cache  object") {
		t.Errorf("the bar reads %q, want the JSON view with the same node selected", got)
	}
}

func TestQuitsOnTheQuitKey(t *testing.T) {
	t.Parallel()

	tm := start(t, testDocument(t), "localhost")

	tm.Type("q")

	// Nothing else stops the program: reaching this point means the key press
	// travelled through the key table, the application and the effect that
	// came back, and that the effect reached the program as a command rather
	// than being handled inside the model.
	tm.WaitFinished(t, teatest.WithFinalTimeout(waitTime))
}
