package presentation

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
)

// What the help screen is held to is that it says everything the key tables
// say, and that it fits on the smallest terminal pino draws in without being
// scrolled. The wording itself is not fixed here: a golden copy of the screen
// would have to be renewed every time a key was added, and renewing it is
// exactly the step at which nobody would notice that it no longer fits.

// The order the headings are read in. It goes from getting about the document
// to changing it to what becomes of the changes, so a reader looking for the
// key they half remember has somewhere to start.
func TestHelpReadsInOneOrder(t *testing.T) {
	t.Parallel()

	want := []string{
		"Move", "Jump", "Search", "Fold", "View", "Edit", "Structure", "History", "File", "Prompt",
	}

	got := make([]string, 0, len(want))
	for _, l := range helpLines() {
		got = append(got, l.Heading)
	}

	if !slices.Equal(got, want) {
		t.Errorf("the headings read %v, want %v", got, want)
	}
}

// The screen is one page on the smallest terminal there is: every row inside
// the width, and the whole of it inside the rows left once the status bar has
// taken its own. Growing past either is what a scrolling help screen would be
// the answer to, and pino does not have one.
func TestHelpFitsTheSmallestTerminal(t *testing.T) {
	t.Parallel()

	body := minHeight - statusBarRows

	rows := helpRows(minWidth)
	if len(rows) > body {
		t.Errorf("the screen is %d rows, want at most %d", len(rows), body)
	}

	for i, row := range rows {
		if got := ansi.StringWidth(row); got > minWidth {
			t.Errorf("row %d is %d columns, want at most %d:\n%s", i, got, minWidth, row)
		}
	}
}

// The same, once it has been coloured. Styling is written into a row as escape
// sequences, so a row that fitted as text and not as drawn would fit only in a
// test.
func TestHelpFitsTheSmallestTerminalWhenDrawn(t *testing.T) {
	t.Parallel()

	drawn := DefaultTheme().RenderHelp(minWidth, minHeight-statusBarRows)

	for i, row := range drawn {
		if got := ansi.StringWidth(row); got != minWidth {
			t.Errorf("row %d is %d columns, want exactly %d", i, got, minWidth)
		}
	}
}

// Mouse reporting makes the wheel available at the cost of the terminal's
// own text selection, so the help screen names both sides of that choice.
func TestHelpOffersMouseScrollingAndTextSelection(t *testing.T) {
	t.Parallel()

	var view string
	for _, row := range helpRows(minWidth) {
		if strings.HasPrefix(row, helpView.String()) {
			view = row

			break
		}
	}

	if view == "" {
		t.Fatal("help has no View row")
	}

	wheel := strings.Index(view, "wheel scroll")
	selecting := strings.Index(view, "m select")

	if wheel < 0 {
		t.Errorf("the View row = %q, want it to offer wheel scrolling", view)
	}

	if selecting < 0 {
		t.Errorf("the View row = %q, want it to offer terminal text selection", view)
	}

	if wheel >= 0 && selecting >= 0 && wheel > selecting {
		t.Errorf("the View row = %q, want wheel scrolling before text selection", view)
	}
}

// The screen is a block of exactly the size it was asked for, the way the
// inspector is: it is stacked above a status bar that has to stay on the last
// row.
func TestRenderHelpFillsTheRoomItIsGiven(t *testing.T) {
	t.Parallel()

	for _, height := range []int{minHeight - statusBarRows, 40} {
		drawn := DefaultTheme().RenderHelp(80, height)

		if len(drawn) != height {
			t.Errorf("RenderHelp(80, %d) drew %d rows, want %d", height, len(drawn), height)
		}

		for i, row := range drawn {
			if got := ansi.StringWidth(row); got != 80 {
				t.Errorf("row %d of RenderHelp(80, %d) is %d columns, want 80", i, height, got)
			}
		}
	}
}

// Every key the tables bind reaches the screen, and no two of them are written
// the same way. This is the whole reason the wording lives beside the
// bindings: a key added to a table and forgotten here would be a key nobody
// could find, and two rows spelled alike would be a screen saying that one key
// in normal mode does two things.
//
// The keys that belong to a prompt are counted apart. Enter is written twice
// on purpose — once for what it does to the document and once for what it does
// to a question — and those are two keys as far as a reader is concerned,
// since only one of the two screens is ever up.
func TestHelpNamesEveryBoundKeyExactlyOnce(t *testing.T) {
	t.Parallel()

	type describedBinding struct {
		group       helpGroup
		keys        string
		description string
		request     string
	}

	bound := make([]describedBinding, 0,
		len(normalBindings)+len(pendingBindings)+len(terminalBindings))

	for _, b := range normalBindings {
		bound = append(bound, describedBinding{
			group: b.Group, keys: b.HelpKeys, description: b.Description,
			request: fmt.Sprintf("%T", b.Action),
		})
	}

	for _, b := range pendingBindings {
		bound = append(bound, describedBinding{
			group: b.Group, keys: b.HelpKeys, description: b.Description,
			request: fmt.Sprintf("%T", b.Action),
		})
	}

	for _, b := range terminalBindings {
		bound = append(bound, describedBinding{
			group: b.Group, keys: b.HelpKeys, description: b.Description,
			request: fmt.Sprintf("terminal action %d", b.Terminal),
		})
	}

	spelt := map[string]string{}
	// The title has no group label; only body rows can hold binding entries.
	body := helpRows(minWidth)[1:]

	for _, b := range bound {
		if b.keys == "" {
			t.Errorf("%s is bound and has no help key", b.request)

			continue
		}

		if first, dup := spelt[b.keys]; dup {
			t.Errorf("%q is written for both %s and %s", b.keys, first, b.request)
		}

		spelt[b.keys] = b.request

		if b.description == "" {
			t.Errorf("%q is on the screen and says nothing about what it does", b.keys)
		}

		written := b.keys + " " + b.description
		matches := 0
		for _, row := range body {
			count := strings.Count(row, written)
			matches += count

			if count > 0 && !strings.HasPrefix(row, pad(b.group.String(), helpLabelWidth)) {
				t.Errorf("%q asks for %s under %s, but is written on row %q",
					b.keys, b.request, b.group, row)
			}
		}

		if matches != 1 {
			t.Errorf("%q asks for %s and is written on the help screen %d times, want 1",
				b.keys, b.request, matches)
		}
	}
}

// The keys that put the screen away are on it, spelled as a reader sees them
// written. A screen offering a way out it does not take, or taking one it does
// not offer, is the way a full-screen mode becomes a trap.
func TestHelpOffersTheKeysThatCloseIt(t *testing.T) {
	t.Parallel()

	title := helpRows(minWidth)[0]

	for _, label := range keyLabels(helpClose) {
		if !strings.Contains(title, label) {
			t.Errorf("the title row is %q, want it to offer %q", title, label)
		}
	}

	if !strings.HasSuffix(title, "close") {
		t.Errorf("the title row is %q, want it to say what those keys do", title)
	}
}

// Each of them closes it, and nothing else does. A key that went on working
// underneath would act on a document nobody can see.
func TestHelpTakesOnlyTheKeysThatCloseIt(t *testing.T) {
	t.Parallel()

	closes := map[string]tea.KeyPressMsg{
		"?":   key('?'),
		"esc": special(tea.KeyEscape),
		"q":   key('q'),
	}

	for name, k := range closes {
		t.Run("closed by "+name, func(t *testing.T) {
			t.Parallel()

			got, terminal, pending := Resolve(k, application.ModeHelp, PendingNone)

			if got != (application.ActionCloseHelp{}) {
				t.Errorf("Resolve(%q, HELP) = %v, want the screen to close", name, got)
			}

			if terminal != TerminalNone {
				t.Errorf("Resolve(%q, HELP) asks for terminal action %v, want none", name, terminal)
			}

			if pending != PendingNone {
				t.Errorf("Resolve(%q, HELP) left %v waiting, want nothing", name, pending)
			}
		})
	}

	ignored := map[string]tea.KeyPressMsg{
		"a movement": key('j'),
		"an edit":    special(tea.KeyEnter),
		"a deletion": key('d'),
		"a prefix":   key('g'),
	}

	for name, k := range ignored {
		t.Run(name+" does nothing", func(t *testing.T) {
			t.Parallel()

			got, terminal, pending := Resolve(k, application.ModeHelp, PendingNone)

			if got != nil {
				t.Errorf("Resolve(%q, HELP) = %v, want nothing", name, got)
			}

			if terminal != TerminalNone {
				t.Errorf("Resolve(%q, HELP) asks for terminal action %v, want none", name, terminal)
			}

			if pending != PendingNone {
				t.Errorf("Resolve(%q, HELP) left %v waiting, want nothing", name, pending)
			}
		})
	}
}

// The key that asks for the screen, from the one mode it is asked from.
func TestTheHelpKeyAsksForTheScreen(t *testing.T) {
	t.Parallel()

	got, terminal, pending := Resolve(key('?'), application.ModeNormal, PendingNone)

	if got != (application.ActionShowHelp{}) {
		t.Errorf("Resolve(?) = %v, want the help screen", got)
	}

	if terminal != TerminalNone {
		t.Errorf("Resolve(?) asks for terminal action %v, want none", terminal)
	}

	if pending != PendingNone {
		t.Errorf("Resolve(?) left %v waiting, want nothing", pending)
	}
}
