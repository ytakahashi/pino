package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// Help is a screen rather than an operation: it is entered, read and left, and
// what it leaves behind has to be the session exactly as it was. What is
// checked here is that nothing about the document can be reached through it,
// and that the one field holding whatever is in progress cannot come to hold
// two things at once.

// standing is everything a session says about itself from outside. Help is
// checked against it before and after, so that a screen said to change nothing
// is checked on the whole session rather than on the parts a test remembered.
type standing struct {
	root     domain.Node
	status   StatusInfo
	rows     int
	cursor   string
	versions int
	at       int
}

func stand(a *App) standing {
	// The mode is left out, being the one thing help is: a standing that
	// carried it could not say that everything else stayed as it was.
	status := a.Status()
	status.Mode = ModeNormal

	return standing{
		root:     a.doc.Root(),
		status:   status,
		rows:     len(a.Frame().Lines),
		cursor:   cursorOf(a),
		versions: len(a.history.entries),
		at:       a.history.cursor,
	}
}

// Opening help puts the session in a mode of its own and touches nothing else.
func TestHelpOpensWithoutDisturbingTheDocument(t *testing.T) {
	t.Parallel()

	a := session(t, testTree(t))
	press(a, ActionMoveNext{})

	before := stand(a)

	press(a, ActionShowHelp{})

	if got := a.Mode(); got != ModeHelp {
		t.Errorf("Mode() = %v after asking for help, want %v", got, ModeHelp)
	}

	if got := stand(a); got != before {
		t.Errorf("help changed the session:\n got %+v\nwant %+v", got, before)
	}
}

// Help asks nothing, which is what sends a key press through the key table
// instead of into a list of choices.
func TestHelpAsksNoQuestion(t *testing.T) {
	t.Parallel()

	a := session(t, testTree(t))
	press(a, ActionShowHelp{})

	if got := a.Prompt(); got.Kind != PromptNone {
		t.Errorf("Prompt() = %+v while help is up, want nothing being asked", got)
	}
}

// Closing gives the document back with everything where it was left.
func TestHelpClosesBackToTheDocument(t *testing.T) {
	t.Parallel()

	a := session(t, testTree(t))
	press(a, ActionMoveNext{}, ActionMoveNext{})

	before := stand(a)

	press(a, ActionShowHelp{}, ActionCloseHelp{})

	if got := a.Mode(); got != ModeNormal {
		t.Errorf("Mode() = %v after closing help, want %v", got, ModeNormal)
	}

	if got := stand(a); got != before {
		t.Errorf("a session that opened and closed help is not the one it was:\n got %+v\nwant %+v", got, before)
	}
}

// A session in the middle of something is in the middle of it still. The
// terminal does not send this while a prompt is up, and the rule holds for an
// Action driven straight at this layer: one field holds what is in progress,
// and help cannot take it from an answer half given.
func TestHelpDoesNotInterruptWhatIsInProgress(t *testing.T) {
	t.Parallel()

	tests := map[string]func(t *testing.T) *App{
		"an edit being typed into": func(t *testing.T) *App {
			t.Helper()

			a := session(t, sample(t))
			standOn(t, a, "/server/host")
			press(a, ActionEdit{})

			return a
		},
		"a question about leaving": func(t *testing.T) *App {
			t.Helper()

			a, _ := saving(t, sample(t))
			editValue(t, a, "/server/ports/0", "8081")
			press(a, ActionQuit{})

			return a
		},
	}

	for name, open := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			a := open(t)

			mode, prompt := a.Mode(), a.Prompt()
			if mode == ModeNormal {
				t.Fatalf("nothing is in progress to interrupt")
			}

			press(a, ActionShowHelp{})

			if got := a.Mode(); got != mode {
				t.Errorf("Mode() = %v after asking for help, want %v", got, mode)
			}

			if got := a.Prompt(); got.Title != prompt.Title || got.Kind != prompt.Kind {
				t.Errorf("the question became %+v, want %+v", got, prompt)
			}
		})
	}
}

// Closing closes help and nothing else. The keys that ask for it mean other
// things elsewhere, so a request arriving while a question is up was meant for
// the question.
func TestClosingHelpLeavesEveryOtherFlowAlone(t *testing.T) {
	t.Parallel()

	a, _ := saving(t, sample(t))
	editValue(t, a, "/server/ports/0", "8081")
	press(a, ActionQuit{})

	before := a.Prompt()

	press(a, ActionCloseHelp{})

	if got := a.Mode(); got != ModeConfirm {
		t.Errorf("Mode() = %v, want the question still up", got)
	}

	if got := a.Prompt(); got.Title != before.Title {
		t.Errorf("Prompt() = %q, want %q", got.Title, before.Title)
	}
}

// Nothing is in progress, so there is nothing to close. It is an Action the
// terminal cannot send from normal mode, and doing nothing is what a session
// reached directly answers with.
func TestClosingHelpThatIsNotOpenDoesNothing(t *testing.T) {
	t.Parallel()

	a := session(t, testTree(t))

	before := stand(a)

	press(a, ActionCloseHelp{})

	if got := a.Mode(); got != ModeNormal {
		t.Errorf("Mode() = %v, want %v", got, ModeNormal)
	}

	if got := stand(a); got != before {
		t.Errorf("closing help that was not open changed the session:\n got %+v\nwant %+v", got, before)
	}
}

// The way out is the way out from here too. A screen that had to be closed
// before the session could be left would be one more thing between a reader
// and the terminal they came from.
func TestQuittingFromHelpLeavesWithNothingToSave(t *testing.T) {
	t.Parallel()

	a := session(t, testTree(t))
	press(a, ActionShowHelp{})

	if got := a.Do(ActionQuit{}); len(got) != 1 || got[0] != (EffectQuit{}) {
		t.Errorf("Do(quit) from help = %v, want the program to stop", got)
	}
}

// Leaving with unsaved work asks, from help as from anywhere. The question
// replaces help rather than standing in front of it: cancelling comes back to
// the document, which is the session the reader still has.
func TestQuittingFromHelpWithUnsavedWorkAsksAndComesBack(t *testing.T) {
	t.Parallel()

	a, files := saving(t, sample(t))
	editValue(t, a, "/server/ports/0", "8081")
	press(a, ActionShowHelp{})

	if got := a.Do(ActionQuit{}); got != nil {
		t.Fatalf("Do(quit) with unsaved work = %v, want a question", got)
	}

	if got := a.Mode(); got != ModeConfirm {
		t.Fatalf("Mode() = %v, want %v", got, ModeConfirm)
	}

	press(a, ActionPromptChoose{Key: 'c'})

	if got := a.Mode(); got != ModeNormal {
		t.Errorf("Mode() = %v after cancelling, want %v", got, ModeNormal)
	}

	if !a.doc.IsDirty() {
		t.Error("the document came back saved, want the work still unsaved")
	}

	if len(files.writes) != 0 {
		t.Errorf("the file was written %d times, want none", len(files.writes))
	}
}
