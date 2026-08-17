package presentation

import (
	"bytes"
	"strings"
	"testing"
	"time"

	teatest "github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/ytakahashi/pino/internal/domain"
)

// waitTime is how long a test allows for the program to draw or to stop. It is
// generous because it bounds a failure rather than a success.
const waitTime = 10 * time.Second

// start puts a document on a pretend terminal and waits for the opening screen
// to be drawn.
//
// The wait is a barrier and not an assertion: keys sent before the program is
// running would be answered by nothing at all. The opening screen is the one
// written whole rather than as a difference, so looking for a piece of it says
// only that the program has started.
func start(t *testing.T, root domain.Node, onFirstScreen string) *teatest.TestModel {
	t.Helper()

	tm := teatest.NewTestModel(
		t,
		NewModel(openApp(t, root), DefaultTheme(), ModelConfig{}),
		teatest.WithInitialTermSize(80, 24),
	)

	teatest.WaitFor(
		t,
		tm.Output(),
		func(out []byte) bool { return bytes.Contains(out, []byte(onFirstScreen)) },
		teatest.WithDuration(waitTime),
	)

	return tm
}

// finalScreen quits and answers the screen the program stopped on, one entry
// per row, taken from the model it ended with.
func finalScreen(t *testing.T, tm *teatest.TestModel) []string {
	t.Helper()

	tm.Type("q")

	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(waitTime)).(Model)
	if !ok {
		t.Fatalf("the program ended with a %T, want a Model", final)
	}

	return rows(t, final)
}

// screenRow is one row of a screen with the filling taken off its right hand
// end, since a row is drawn out to the width it has.
func screenRow(screen []string, i int) string {
	if i < 0 || i >= len(screen) {
		return ""
	}

	return strings.TrimRight(screen[i], " ")
}

func statusRow(screen []string) string { return screenRow(screen, len(screen)-1) }
