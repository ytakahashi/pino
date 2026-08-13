package presentation

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Editing a document through the terminal: the keys that open a prompt, where
// they go while one is open, and what the screen looks like meanwhile.
//
// These drive the model rather than the session, so what they check is the
// wiring: the same edits are checked for what they do to a document one layer
// down.

func TestTypingAValueChangesTheDocument(t *testing.T) {
	t.Parallel()

	// Down to the string, then Enter to type over it.
	m := press(t, sized(t, openTestApp(t), 60, 12), key('j'), enterKey)

	if got := band(t, m); len(got) == 0 || !strings.Contains(got[1], "localhost") {
		t.Fatalf("the band reads %q, want the value being edited", got)
	}

	m = press(t, m, key('x'), enterKey)

	if got := rows(t, m); !strings.Contains(got[1], `"host": "localhostx"`) {
		t.Errorf("the document reads %q, want the value that was typed", got[1])
	}

	if got := band(t, m); got != nil {
		t.Errorf("the band is still up: %q", got)
	}

	if got := statusRowOf(t, m); !strings.Contains(got, "modified") {
		t.Errorf("the bar reads %q, want it to report an unsaved change", got)
	}
}

func TestTheKeysOfTheDocumentDoNotReachThePrompt(t *testing.T) {
	t.Parallel()

	// j moves in the document and types a letter in a box. Which of the two it
	// is is decided by what is on screen, not by the key.
	m := press(t, sized(t, openTestApp(t), 60, 12), key('j'), enterKey, key('j'))

	if got := band(t, m); !strings.Contains(got[1], "localhostj") {
		t.Errorf("the band reads %q, want the letter typed into the box", got)
	}

	m = press(t, m, enterKey)

	if got := rows(t, m); !strings.Contains(got[1], `"host": "localhostj"`) {
		t.Errorf("the document reads %q", got[1])
	}
}

func TestEscapeLeavesTheDocumentAsItWas(t *testing.T) {
	t.Parallel()

	// Standing on the value first: what is being checked is that the edit left
	// nothing behind, not that the cursor stayed at the top.
	m := press(t, sized(t, openTestApp(t), 60, 12), key('j'))
	before := rows(t, m)

	m = press(t, m, enterKey, key('x'), escapeKey)

	if got := band(t, m); got != nil {
		t.Errorf("the band is still up after Esc: %q", got)
	}

	if got := rows(t, m); !equalRows(got, before) {
		t.Errorf("the screen changed although the edit was abandoned:\n%v\n%v", got, before)
	}
}

func TestAnAnswerThatCannotBeCommittedStaysOnTheBand(t *testing.T) {
	t.Parallel()

	// Down twice to the number, then a letter into it.
	m := press(t, sized(t, openTestApp(t), 60, 12), key('j'), key('j'), enterKey, key('x'))

	got := band(t, m)
	if len(got) != 3 {
		t.Fatalf("the band drew %d rows, want a rule, the value and the reason", len(got))
	}

	if !strings.Contains(got[2], "not a JSON number") {
		t.Errorf("the last row reads %q, want why the answer was refused", got[2])
	}

	// Enter is refused as well, and what was typed is still there to correct.
	m = press(t, m, enterKey)

	if got := band(t, m); len(got) != 3 || !strings.Contains(got[1], "8080x") {
		t.Errorf("the band reads %q, want the answer still in the box", got)
	}

	if got := statusRowOf(t, m); strings.Contains(got, "modified") {
		t.Errorf("the bar reads %q, want no change to the document", got)
	}
}

func TestChoosingATypeFromTheListChangesTheValue(t *testing.T) {
	t.Parallel()

	m := press(t, sized(t, openTestApp(t), 60, 12), key('j'), key('j'), key('t'))

	got := band(t, m)
	if len(got) != 4 {
		t.Fatalf("the band drew %d rows, want a rule, a title and two rows of types", len(got))
	}

	if !strings.Contains(got[1], "type") || !strings.Contains(got[2], "[s] string") {
		t.Errorf("the band reads %q, want the list of types", got)
	}

	m = press(t, m, key('b'))

	if got := rows(t, m); !strings.Contains(got[2], `"port": false`) {
		t.Errorf("the document reads %q, want the type that was chosen", got[2])
	}

	if got := band(t, m); got != nil {
		t.Errorf("the band is still up: %q", got)
	}
}

func TestAKeyTheListDoesNotOfferLeavesItUp(t *testing.T) {
	t.Parallel()

	m := press(t, sized(t, openTestApp(t), 60, 12), key('j'), key('t'), key('q'))

	if got := band(t, m); len(got) == 0 {
		t.Error("a key the list does not offer closed it")
	}

	if got := statusRowOf(t, m); !strings.Contains(got, "EDIT") {
		t.Errorf("the bar reads %q, want the session still in edit", got)
	}
}

func TestTheBandTakesItsRowsFromTheDocument(t *testing.T) {
	t.Parallel()

	m := sized(t, openTestApp(t), 60, 12)
	before := m.layout().BodyHeight

	m = press(t, m, key('j'), enterKey)

	l := m.layout()
	if l.PromptHeight <= 0 {
		t.Fatal("the band asked for no rows")
	}

	if l.BodyHeight != before-l.PromptHeight {
		t.Errorf("the document has %d rows, want %d less the %d the band took",
			l.BodyHeight, before, l.PromptHeight)
	}

	// And the session is told, since it follows the cursor with the window.
	if m.reported != l.BodyHeight {
		t.Errorf("the session was told %d rows, want %d", m.reported, l.BodyHeight)
	}

	m = press(t, m, escapeKey)

	if got := m.layout().BodyHeight; got != before {
		t.Errorf("the document has %d rows once the band has gone, want the %d it had", got, before)
	}

	if m.reported != before {
		t.Errorf("the session was told %d rows, want %d", m.reported, before)
	}
}

func TestABoxThatGrowsTakesAnotherRow(t *testing.T) {
	t.Parallel()

	m := press(t, sized(t, openTestApp(t), 60, 12), key('j'), enterKey)
	before := m.layout()

	// Ctrl+j is a newline in a string, which is the one answer that can be
	// more than a row.
	m = press(t, m, ctrl('j'))

	if got := m.layout(); got.PromptHeight != before.PromptHeight+1 {
		t.Errorf("the band takes %d rows, want one more than the %d it did",
			got.PromptHeight, before.PromptHeight)
	}

	if got := m.layout().BodyHeight; got != before.BodyHeight-1 {
		t.Errorf("the document has %d rows, want one less than the %d it had",
			got, before.BodyHeight)
	}

	if m.reported != m.layout().BodyHeight {
		t.Errorf("the session was told %d rows, want %d", m.reported, m.layout().BodyHeight)
	}
}

func TestCtrlCLeavesFromAnyPrompt(t *testing.T) {
	t.Parallel()

	opens := map[string][]tea.KeyPressMsg{
		"a box being typed into": {key('j'), enterKey},
		"a list of types":        {key('j'), key('t')},
	}

	for name, keys := range opens {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m := press(t, sized(t, openTestApp(t), 60, 12), keys...)

			if band(t, m) == nil {
				t.Fatal("no prompt opened")
			}

			_, cmd := m.Update(ctrl('c'))
			if cmd == nil {
				t.Fatal("ctrl+c produced no command, want a quit")
			}

			if msg := cmd(); !isQuit(msg) {
				t.Errorf("ctrl+c produced %T, want tea.QuitMsg", msg)
			}
		})
	}
}

func TestTheBandIsDrawnBetweenTheDocumentAndTheBar(t *testing.T) {
	t.Parallel()

	m := press(t, sized(t, openTestApp(t), 60, 12), key('j'), enterKey)

	drawn := rows(t, m)
	if len(drawn) != 12 {
		t.Fatalf("the screen has %d rows, want 12", len(drawn))
	}

	l := m.layout()

	// The rule divides the document from the band, as it does the inspector.
	rule := drawn[l.BodyHeight]
	if !strings.HasPrefix(rule, strings.Repeat("─", 10)) {
		t.Errorf("the row under the document is %q, want a rule", rule)
	}

	if got := statusRowOf(t, m); !strings.Contains(got, "EDIT") {
		t.Errorf("the bar reads %q, want the mode the prompt put the session in", got)
	}
}

func TestKeyboardUndoAndRedoRestoreVersions(t *testing.T) {
	t.Parallel()

	m := press(t, sized(t, openTestApp(t), 60, 12), key('j'), key('j'), enterKey, key('1'), enterKey)

	if got := rows(t, m); !strings.Contains(got[2], "80801") {
		t.Fatalf("the document reads %q, want the value that was typed", got[2])
	}

	m = press(t, m, key('u'))

	if got := rows(t, m); !strings.Contains(got[2], "8080") || strings.Contains(got[2], "80801") {
		t.Errorf("the document reads %q after undo, want the value that was there", got[2])
	}

	if got := statusRowOf(t, m); strings.Contains(got, "modified") {
		t.Errorf("the bar reads %q, want the unsaved mark gone", got)
	}

	m = press(t, m, ctrl('r'))

	if got := rows(t, m); !strings.Contains(got[2], "80801") {
		t.Errorf("the document reads %q after redo, want the edit back", got[2])
	}
}

// The screen is exactly as tall as the terminal however little is left for the
// document: the band is fitted to the rows the layout set aside for it, so a
// question asked on the smallest terminal pino draws in cannot push the status
// bar off the bottom.
func TestTheScreenIsWholeWithABandOnTheSmallestTerminal(t *testing.T) {
	t.Parallel()

	for name, keys := range map[string][]tea.KeyPressMsg{
		"the JSON view":     {key('j'), key('t')},
		"the tree view too": {tabKey, key('j'), key('t')},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m := press(t, sized(t, openApp(t, longDocument(t)), minWidth, minHeight), keys...)

			drawn := rows(t, m)
			if len(drawn) != minHeight {
				t.Fatalf("the screen has %d rows, want %d", len(drawn), minHeight)
			}

			if got := m.layout(); got.BodyHeight < 0 || got.PromptHeight > minHeight {
				t.Errorf("the screen is divided as %+v", got)
			}

			if !strings.Contains(drawn[len(drawn)-1], "EDIT") {
				t.Errorf("the last row is %q, want the status bar", drawn[len(drawn)-1])
			}
		})
	}
}

// A value is not changed by being looked at.
//
// This is the whole chain: the session spells the value, the widget holds the
// spelling, and the session reads it back. The widgets run what they are given
// through a sanitizer — a tab becomes four spaces, a control character is
// dropped — so a value handed to one as it stands would come back short of
// what it held, and pressing Enter twice would commit the loss.
func TestOpeningAValueAndCommittingItChangesNothing(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"a tab":              "a\tb",
		"a carriage return":  "a\rb",
		"a null":             "a\x00b",
		"a bell":             "a\x07b",
		"delete":             "a\x7fb",
		"a C1 control":       "a\u0085b",
		"a replacement char": "a\ufffdb",
		"a quotation mark":   `he said "hi"`,
		"a backslash":        `C:\Users\pino`,
		"a line break":       "a\nb",
		"plain text":         "localhost",
	}

	for name, value := range values {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m := press(t, sized(t, openApp(t, awkwardDocument(t, value)), 60, 12), key('j'))
			before := rows(t, m)

			// Open the box and commit what is in it, without typing.
			m = press(t, m, enterKey, enterKey)

			if got := statusRowOf(t, m); strings.Contains(got, "modified") {
				t.Errorf("the document was changed by being looked at: %q", got)
			}

			if got := rows(t, m); !equalRows(got, before) {
				t.Errorf("the screen changed:\n%q\n%q", got[1], before[1])
			}

			if got := band(t, m); got != nil {
				t.Errorf("the band is still up: %q", got)
			}
		})
	}
}

func TestPastingIntoTheBoxInsertsClipboardText(t *testing.T) {
	t.Parallel()

	m := press(t, sized(t, openTestApp(t), 60, 12), key('j'), enterKey)
	before := m.layout()

	next, _ := m.Update(tea.PasteMsg{Content: "!\nmore"})

	m, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if got := band(t, m); !strings.Contains(got[1], "localhost!") {
		t.Errorf("the band reads %q, want what was pasted", got)
	}

	// A paste of several lines grows the box, and the document gives up the
	// rows: what pasting produces is answered the same way typing is.
	if got := m.layout(); got.PromptHeight <= before.PromptHeight {
		t.Errorf("the band takes %d rows, want more than the %d it did",
			got.PromptHeight, before.PromptHeight)
	}

	if m.reported != m.layout().BodyHeight {
		t.Errorf("the session was told %d rows, want %d", m.reported, m.layout().BodyHeight)
	}
}

func TestPastingWithNoBoxOpenDoesNothing(t *testing.T) {
	t.Parallel()

	// There is nothing in a document to paste into until pino can copy a
	// subtree, and a stray paste must not be read as the keys it holds.
	m := sized(t, openTestApp(t), 60, 12)
	before := rows(t, m)

	next, cmd := m.Update(tea.PasteMsg{Content: "jjj"})

	if cmd != nil {
		t.Error("a paste with no box open produced a command")
	}

	assertSameSession(t, next, m)

	if got := rows(t, next.(Model)); !equalRows(got, before) {
		t.Error("a paste with no box open changed the screen")
	}
}
