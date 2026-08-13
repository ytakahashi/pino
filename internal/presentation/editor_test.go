package presentation

import (
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/application"
)

// The box an answer is typed into: what it starts with, what it accepts, and
// how much of the screen it asks for.

func TestTheBoxStartsFromTheValueItWasGiven(t *testing.T) {
	t.Parallel()

	for name, multiline := range map[string]bool{"a number": false, "a string": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := box("8080", multiline).Value(); got != "8080" {
				t.Errorf("the box holds %q, want the value it was given", got)
			}
		})
	}
}

func TestTheBoxTakesWhatIsTypedIntoIt(t *testing.T) {
	t.Parallel()

	for name, multiline := range map[string]bool{"a number": false, "a string": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// The caret starts at the end of what was seeded, so typing
			// continues the value rather than going in front of it.
			e := box("80", multiline).Update(key('9'))

			if got := e.Value(); got != "809" {
				t.Errorf("the box holds %q, want %q", got, "809")
			}
		})
	}
}

func TestOnlyAStringCanBeGivenANewline(t *testing.T) {
	t.Parallel()

	// A widget that cannot make a newline is how "no newline belongs here" is
	// enforced: the grammar is not checked again on this side.
	if got := box("ab", true).InsertNewline().Value(); got != "ab\n" {
		t.Errorf("a string holds %q after a newline, want %q", got, "ab\n")
	}

	if got := box("12", false).InsertNewline().Value(); got != "12" {
		t.Errorf("a number holds %q after a newline, want it unchanged", got)
	}
}

func TestTheBoxIsAsTallAsWhatIsInIt(t *testing.T) {
	t.Parallel()

	if got := box("8080", false).Rows(); got != 1 {
		t.Errorf("a number takes %d rows, want 1", got)
	}

	if got := box("one\ntwo", true).Rows(); got != 2 {
		t.Errorf("two lines take %d rows, want 2", got)
	}

	// Past the share of the screen a prompt may take, the box scrolls instead
	// of growing: what may be typed is not decided by how tall a band is.
	tall := box(strings.Repeat("line\n", maxInputRows+3), true)

	if got := tall.Rows(); got != maxInputRows {
		t.Errorf("a long value takes %d rows, want %d", got, maxInputRows)
	}

	if got := len(strings.Split(tall.Value(), "\n")); got != maxInputRows+4 {
		t.Errorf("the box holds %d lines, want the %d it was given", got, maxInputRows+4)
	}
}

func TestTheBoxGoesOnTakingLinesOnceItIsFull(t *testing.T) {
	t.Parallel()

	// The height of a band on screen must not decide what a document may
	// contain: a JSON string may hold any number of newlines.
	e := box(strings.Repeat("line\n", maxInputRows), true)
	for range 3 {
		e = e.InsertNewline()
	}

	if got := strings.Count(e.Value(), "\n"); got != maxInputRows+3 {
		t.Errorf("the box holds %d newlines, want %d", got, maxInputRows+3)
	}
}

func TestNoBoxIsNoRowsAndNoText(t *testing.T) {
	t.Parallel()

	// The zero value is what the model holds while nothing is being typed
	// into, and every one of these has to answer for it without a widget.
	var none editor

	if got := none.Rows(); got != 0 {
		t.Errorf("Rows() = %d with no box, want 0", got)
	}

	if got := none.Value(); got != "" {
		t.Errorf("Value() = %q with no box, want empty", got)
	}

	if got := none.View(); got != nil {
		t.Errorf("View() = %q with no box, want nothing", got)
	}

	none = none.SetWidth(20).Update(key('x')).InsertNewline()

	if got := none.Value(); got != "" {
		t.Errorf("Value() = %q after typing into no box, want empty", got)
	}
}

func TestTheBoxDrawsWhatIsInIt(t *testing.T) {
	t.Parallel()

	if got := box("8080", false).View(); len(got) != 1 || !strings.Contains(got[0], "8080") {
		t.Errorf("the box drew %q, want the value on one row", got)
	}

	// As many rows as it is tall, so that the band can lay them out.
	if got := box("one\ntwo", true).View(); len(got) != 2 {
		t.Errorf("the box drew %d rows, want 2", len(got))
	}
}

// What the box draws and what it says it needs are the same number. The screen
// is divided from the second before the first is asked for, so a box drawing a
// row more than it asked for would push the status bar off the bottom.
func TestTheBoxDrawsAsManyRowsAsItAsksFor(t *testing.T) {
	t.Parallel()

	boxes := map[string]editor{
		"a number":       box("8080", false),
		"a line":         box("localhost", true),
		"two lines":      box("one\ntwo", true),
		"more than fits": box(strings.Repeat("line\n", maxInputRows+2), true),
		"nothing at all": {},
	}

	for name, e := range boxes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got, want := len(e.View()), e.Rows(); got != want {
				t.Errorf("the box drew %d rows and asked for %d", got, want)
			}
		})
	}
}

// The box holds what it was given, character for character.
//
// This is what keeps a value from being changed by having been looked at. The
// widgets run everything through a sanitizer of their own — a tab becomes four
// spaces, a control character is dropped — so anything that cannot survive it
// has to reach the box already spelled as an escape.
func TestTheBoxHoldsTheSpellingItWasGiven(t *testing.T) {
	t.Parallel()

	for name, spelling := range awkwardSpellings() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for shape, multiline := range map[string]bool{"a string": true, "a number": false} {
				if !multiline && strings.Contains(spelling, "\n") {
					// A number is never spelled with a break in it.
					continue
				}

				if got := box(spelling, multiline).Value(); got != spelling {
					t.Errorf("%s: the box holds %q, want %q", shape, got, spelling)
				}
			}
		})
	}
}

// A value with more rows than the widget will hold is taken on one line
// instead, since a box that took it as far as its own limit would let someone
// who only looked at a value commit it short.
func TestAValueTooTallForTheBoxIsTakenOnOneLine(t *testing.T) {
	t.Parallel()

	rows := strings.Repeat("row\n", 12000)
	line := strings.ReplaceAll(rows, "\n", `\n`)

	e, ok := newEditor(
		DefaultTheme(),
		application.EffectBeginInput{Text: rows, OneLine: line, Multiline: true},
		40,
	)
	if !ok {
		t.Fatal("the box did not take the value on one line either")
	}

	if got := e.Value(); got != line {
		t.Errorf("the box holds %d characters, want the %d of the value on one line",
			len(got), len(line))
	}

	// One line of sixty thousand characters still wraps, so what it costs the
	// document is bounded the same way any other long value is.
	if got := e.Rows(); got != maxInputRows {
		t.Errorf("the box takes %d rows, want the %d a box may have", got, maxInputRows)
	}
}

// A box that did not take the value says so.
//
// Nothing the session sends should reach this: the widget is given limits past
// anything a terminal can draw, and a value with more lines than it will hold
// arrives with a one-line spelling to fall back on. The report is what keeps
// the failure from being silent if one ever does — the model ends the edit
// rather than letting Enter commit whatever the box managed to hold.
func TestABoxSaysWhenItCouldNotTakeTheValue(t *testing.T) {
	t.Parallel()

	rows := strings.Repeat("row\n", 12000)

	// A value past the widget's own limit on lines, offered with no spelling
	// that escapes it: both attempts come up short.
	_, ok := newEditor(
		DefaultTheme(),
		application.EffectBeginInput{Text: rows, OneLine: rows, Multiline: true},
		40,
	)

	if ok {
		t.Error("the box reported holding a value it cannot hold")
	}

	// And the ordinary case says so too.
	if _, ok := newEditor(
		DefaultTheme(),
		application.EffectBeginInput{Text: "8080", OneLine: "8080"},
		40,
	); !ok {
		t.Error("the box reported not holding a value it holds")
	}
}

// A single line of any length is held whole. It is one line, so the widget's
// limit on lines cannot reach it, and the limit on drawn rows is past what a
// terminal can show.
func TestALongLineIsHeldWhole(t *testing.T) {
	t.Parallel()

	// Longer than the box is wide by a wide margin, so it wraps many times
	// over: wrapped rows are what the widget counts.
	line := strings.Repeat("x", 40*5000)

	e, ok := newEditor(
		DefaultTheme(),
		application.EffectBeginInput{Text: line, OneLine: line, Multiline: true},
		40,
	)

	if !ok {
		t.Fatal("the box did not take a long line")
	}

	if got := e.Value(); got != line {
		t.Errorf("the box holds %d characters of %d", len(got), len(line))
	}
}
