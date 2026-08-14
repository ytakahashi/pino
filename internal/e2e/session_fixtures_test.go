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

// screenObserver keeps every screen the model drew, in the order it drew them,
// and wakes whoever is waiting when another arrives.
//
// The terminal's own output cannot answer for this: it carries the cells that
// changed rather than screens, so whether a piece of text arrives in one run
// depends on what stood in its place before it. finalScreen says the same of
// the screen a program stops on.
type screenObserver struct {
	mu      sync.Mutex
	screens []string
	changed chan struct{}
}

func newScreenObserver() *screenObserver {
	return &screenObserver{changed: make(chan struct{}, 1)}
}

func (o *screenObserver) record(content string) {
	o.mu.Lock()
	o.screens = append(o.screens, content)
	o.mu.Unlock()

	// A waiter that has not gone back to sleep yet needs no second nudge: it
	// reads every screen it has not seen before waiting again, so one wake-up
	// left pending stands for however many screens arrived.
	select {
	case o.changed <- struct{}{}:
	default:
	}
}

// at is the screen drawn i-th, without styling and one entry per row, and
// whether that many have been drawn.
func (o *screenObserver) at(i int) ([]string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if i >= len(o.screens) {
		return nil, false
	}

	return strings.Split(ansi.Strip(o.screens[i]), "\n"), true
}

// screenWaiter reads the screens a program drew, once each and in the order
// they were drawn.
//
// Reading in order is what makes a wait repeatable: which screen answers it
// follows from the sequence the program drew, rather than from which of them
// the test happened to look at while the program ran on.
//
// What a wait sees is the screens drawn since the wait before it settled, and
// none from earlier. That is a narrower window than "whatever is on hand" and
// not a promise that the state described has been reached: most states a
// scenario asks about cannot be told apart from ones the program was in
// earlier — the screen before an edit reads as "8080 and nothing modified"
// exactly as the screen after undoing that edit does. Each wait is still
// written to describe a state that the keys since the last wait produce.
type screenWaiter struct {
	observer *screenObserver
	next     int
}

// wait answers the first screen not yet read that ready accepts.
func (w *screenWaiter) wait(t *testing.T, ready func(screen []string) bool) []string {
	t.Helper()

	timer := time.NewTimer(waitTime)
	defer timer.Stop()

	var last []string

	for {
		screen, drawn := w.observer.at(w.next)
		if drawn {
			w.next++
			last = screen

			if ready(screen) {
				return screen
			}

			continue
		}

		select {
		case <-w.observer.changed:
		case <-timer.C:
			t.Fatalf("no screen reached the state waited for; the last one drawn was:\n%s",
				strings.Join(last, "\n"))
		}
	}
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

// start opens the document through the assembly the command line uses and
// waits for the opening screen to be drawn.
//
// The wait is a barrier and not an assertion: keys sent before the program is
// running would be answered by nothing at all. It is made twice over because
// the two ways of making it say different things. The terminal is searched
// here and nowhere else, since it is the only place that reports that the
// bytes went out at all; finalScreen says why it is not searched for anything
// more than that. The waiter is then walked to that same screen, so that a
// scenario begins reading where the document first appeared rather than at
// whatever the program drew while it was starting.
//
// Every scenario is observed, the wrapper being transparent, so that there is
// one way to start a program rather than one for each kind of assertion.
func start(t *testing.T, onFirstScreen string) (*teatest.TestModel, *screenWaiter) {
	t.Helper()

	model, err := cli.NewProgramModel(writeConfig(t))
	if err != nil {
		t.Fatalf("NewProgramModel() = %v", err)
	}

	observer := newScreenObserver()

	tm := teatest.NewTestModel(
		t,
		observedModel{inner: model, observer: observer},
		teatest.WithInitialTermSize(80, 24),
	)

	teatest.WaitFor(
		t,
		tm.Output(),
		func(out []byte) bool { return bytes.Contains(out, []byte(onFirstScreen)) },
		teatest.WithDuration(waitTime),
	)

	waiter := &screenWaiter{observer: observer}

	waiter.wait(t, func(screen []string) bool {
		return strings.Contains(strings.Join(screen, "\n"), onFirstScreen)
	})

	return tm, waiter
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
