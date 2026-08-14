package e2e

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/ytakahashi/pino/internal/cli"
)

// waitTime is how long a scenario allows for the program to draw or to stop.
// It is generous because it bounds a failure rather than a success.
const waitTime = 10 * time.Second

// config is the document the scenarios open.
//
// It is indented with four spaces rather than the two a test would otherwise
// write, so that a bar reading "indent:4" can only have come from the file:
// no double here supplies a layout, and nothing in pino would guess that one.
const config = `{
    "server": {
        "cache": {
            "ttl": 60
        },
        "host": "localhost"
    },
    "port": 8080
}
`

// writeConfig puts the document on disk and answers the path to it.
//
// The name matters: it is what the bar shows, so a scenario that reads it back
// is reading a real file's name rather than one a test handed the session.
func writeConfig(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")

	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}

	return path
}

// start opens the document through the assembly the command line uses and
// waits for the opening screen to be drawn.
//
// The wait is a barrier and not an assertion: keys sent before the program is
// running would be answered by nothing at all. The opening screen is the one
// written whole rather than as a difference, so looking for a piece of it says
// only that the program has started.
func start(t *testing.T, onFirstScreen string) *teatest.TestModel {
	t.Helper()

	model := programModel(t)

	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(80, 24))

	teatest.WaitFor(
		t,
		tm.Output(),
		func(out []byte) bool { return bytes.Contains(out, []byte(onFirstScreen)) },
		teatest.WithDuration(waitTime),
	)

	return tm
}

func programModel(t *testing.T) tea.Model {
	t.Helper()

	model, err := cli.NewProgramModel(writeConfig(t))
	if err != nil {
		t.Fatalf("NewProgramModel() = %v", err)
	}

	return model
}

// screenObserver records complete screens rendered by the model while a
// program is running. Terminal output cannot serve this purpose because it is
// a stream of cell differences rather than a sequence of complete screens.
type screenObserver struct {
	mu      sync.RWMutex
	content string
	changed chan struct{}
}

func newScreenObserver() *screenObserver {
	return &screenObserver{changed: make(chan struct{}, 1)}
}

func (o *screenObserver) record(content string) {
	o.mu.Lock()
	o.content = content
	o.mu.Unlock()

	select {
	case o.changed <- struct{}{}:
	default:
	}
}

func (o *screenObserver) screen() []string {
	o.mu.RLock()
	content := o.content
	o.mu.RUnlock()

	return strings.Split(ansi.Strip(content), "\n")
}

// observedModel delegates every operation to the real program model and
// records the complete view after each state transition.
type observedModel struct {
	inner    tea.Model
	observer *screenObserver
}

func (m observedModel) Init() tea.Cmd {
	m.observer.record(m.inner.View().Content)

	return m.inner.Init()
}

func (m observedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.inner.Update(msg)
	m.observer.record(next.View().Content)

	return observedModel{inner: next, observer: m.observer}, cmd
}

func (m observedModel) View() tea.View { return m.inner.View() }

func startObserved(t *testing.T, onFirstScreen string) (*teatest.TestModel, *screenObserver) {
	t.Helper()

	observer := newScreenObserver()
	tm := teatest.NewTestModel(
		t,
		observedModel{inner: programModel(t), observer: observer},
		teatest.WithInitialTermSize(80, 24),
	)

	teatest.WaitFor(
		t,
		tm.Output(),
		func(out []byte) bool { return bytes.Contains(out, []byte(onFirstScreen)) },
		teatest.WithDuration(waitTime),
	)

	return tm, observer
}

func waitForScreen(t *testing.T, observer *screenObserver, ready func([]string) bool) []string {
	t.Helper()

	timer := time.NewTimer(waitTime)
	defer timer.Stop()

	for {
		screen := observer.screen()
		if ready(screen) {
			return screen
		}

		select {
		case <-observer.changed:
		case <-timer.C:
			t.Fatalf("screen did not reach the expected state:\n%s", strings.Join(screen, "\n"))
		}
	}
}

// finalScreen quits and answers the screen the program stopped on, one entry
// per row, taken from the model it ended with.
//
// What reached the terminal is never searched. A terminal is sent only the
// cells that changed, so whether a piece of text arrives as one run depends on
// what was on screen before it; searching that stream tests the diffing rather
// than pino, and answers differently on different machines.
func finalScreen(t *testing.T, tm *teatest.TestModel) []string {
	t.Helper()

	tm.Type("q")

	final := tm.FinalModel(t, teatest.WithFinalTimeout(waitTime))

	return screenOf(final)
}

// screenOf is what a model draws, without styling, one entry per row.
func screenOf(m tea.Model) []string {
	return strings.Split(ansi.Strip(m.View().Content), "\n")
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
