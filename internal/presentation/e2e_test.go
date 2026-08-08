package presentation

import (
	"bytes"
	"testing"
	"time"

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
