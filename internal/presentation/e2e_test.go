package presentation

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
)

// The end-to-end test drives a real Bubble Tea program writing to a pretend
// terminal, which is the only way to exercise what the pieces tested
// separately leave out: that a program built from this model starts, draws the
// document to the screen, and stops on the key that says so.
//
// It runs against doubles for the ports rather than the real adapters, since
// this layer is not allowed to name them. What it is testing is the loop, not
// the parsing.

// waitTime is how long the test allows for a frame to appear or for the
// program to stop. It is generous because it bounds a failure rather than a
// success: a working program passes as soon as it has drawn.
const waitTime = 10 * time.Second

// waitForRedraw blocks until the program writes something holding want.
//
// Each call reads on from where the last one stopped, and a terminal is only
// sent what has changed, so what is waited for has to be text the keystroke
// causes to be written afresh. Moving the selection repaints the rows it
// leaves and lands on, which is what makes their contents usable here.
func waitForRedraw(t *testing.T, tm *teatest.TestModel, want string) {
	t.Helper()

	teatest.WaitFor(
		t,
		tm.Output(),
		func(out []byte) bool { return bytes.Contains(out, []byte(want)) },
		teatest.WithDuration(waitTime),
	)
}

// Reading a document, done the way a person would: a real program, a real
// terminal, and keystrokes arriving one at a time. What this adds to the tests
// either side of the key table is that the whole run of key to Action to state
// to screen holds together, prefix keys and the status bar included.
func TestReadsADocument(t *testing.T) {
	t.Parallel()

	tm := teatest.NewTestModel(
		t,
		NewModel(openApp(t, nestedDocument(t)), DefaultTheme()),
		teatest.WithInitialTermSize(80, 24),
	)

	// The document has to be drawn before any key is sent, or it would be
	// answered by a model that has not yet been told the size of the terminal.
	waitForRedraw(t, tm, "localhost")

	// Down twice, into the nested object, then in to its first member and
	// back out to it. Each row is repainted as it takes and loses the
	// selection, which is what makes the four keys visible from out here.
	tm.Type("j")
	waitForRedraw(t, tm, `"server": {`)

	tm.Type("j")
	waitForRedraw(t, tm, `"cache": {`)

	tm.Type("l")
	waitForRedraw(t, tm, `"ttl": 60`)

	tm.Type("h")
	waitForRedraw(t, tm, `"cache": {`)

	// A prefix key on its own draws nothing but says so on the bar. Sent
	// separately from the key that completes it, since the two arriving
	// together would never show the waiting.
	tm.Type("z")
	waitForRedraw(t, tm, "indent:2  z")

	// And the key that completes it folds the document down to its shape,
	// which is text nothing else on the screen could have produced.
	tm.Type("M")
	waitForRedraw(t, tm, "…}")

	// Everything back.
	tm.Type("zR")
	waitForRedraw(t, tm, "localhost")

	// And to the last node, which is not the last row.
	tm.Type("G")
	waitForRedraw(t, tm, `"port": 8080`)

	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(waitTime))
}

func TestQuitsOnTheQuitKey(t *testing.T) {
	t.Parallel()

	tm := teatest.NewTestModel(
		t,
		NewModel(openTestApp(t), DefaultTheme()),
		teatest.WithInitialTermSize(80, 24),
	)

	// The document has to be on the screen before the keystroke is sent:
	// arriving first, it would be answered by a model that has not yet been
	// told the size of the terminal, and the test would pass without the
	// drawing ever having happened.
	teatest.WaitFor(
		t,
		tm.Output(),
		func(out []byte) bool { return bytes.Contains(out, []byte("localhost")) },
		teatest.WithDuration(waitTime),
	)

	tm.Type("q")

	// Nothing else stops the program: reaching this point means the key press
	// travelled through the key table, the application and the effect that
	// came back, and that the effect reached the program as a command rather
	// than being handled inside the model.
	tm.WaitFinished(t, teatest.WithFinalTimeout(waitTime))
}

// Switching between the two views, done the same way. What this adds to the
// tests either side of it is that the whole run holds together at once: Tab
// reaches the key table, the session swaps renderers, the screen is laid out
// afresh with an inspector that costs the document rows, and the folded set
// crosses in both directions.
//
// Eighty columns puts the inspector under the tree rather than beside it,
// which is the arrangement that changes how many rows the document has. The
// one beside it changes only columns and is covered where the screen is
// assembled.
//
// One thing is waited for per keystroke, since a keystroke repaints once and
// each wait reads on from where the last one stopped. What each one waits for
// is text that keystroke had to have produced.
func TestSwitchesBetweenTheViews(t *testing.T) {
	t.Parallel()

	tm := teatest.NewTestModel(
		t,
		NewModel(openApp(t, nestedDocument(t)), DefaultTheme()),
		teatest.WithInitialTermSize(80, 24),
	)

	waitForRedraw(t, tm, "localhost")

	// Down to a node well inside the document, so that keeping the selection
	// across a switch is more than staying on the root.
	tm.Type("j")
	waitForRedraw(t, tm, `"server": {`)

	tm.Type("j")
	waitForRedraw(t, tm, `"cache": {`)

	// Folded away here, in the JSON view.
	tm.Type("h")
	waitForRedraw(t, tm, "…},")

	// Tab, sent as a terminal reports it: Type would fill in the text of the
	// keystroke, which a terminal does not do for a key standing for no
	// character, and the table matches on how a key is spelled.
	//
	// The tree draws the same node, still folded, with a marker and a count of
	// what it holds. Nothing but the tree renderer working from the session's
	// own folded set could have produced that row.
	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab})
	waitForRedraw(t, tm, "▶ cache {1}")

	// And unfolded again from this side, which draws the member the tree's
	// way: a name without quotes, where the JSON view writes one with them.
	tm.Type("l")
	waitForRedraw(t, tm, "ttl: 60")

	// The inspector follows the selection: stepping from the container onto
	// the member it holds turns the field that counted its children into the
	// one that shows a value.
	tm.Type("j")
	waitForRedraw(t, tm, "Value     60")

	// Back to the document as it is written, unfolded as the tree left it.
	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab})
	waitForRedraw(t, tm, `"ttl": 60`)

	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(waitTime))
}
