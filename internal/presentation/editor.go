package presentation

import (
	"math"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application"
)

// maxInputRows is how tall the box an answer is typed into may grow.
//
// It bounds what the prompt takes from the document, not what can be typed: a
// JSON string may hold any number of newlines, so a taller answer scrolls
// inside the box rather than being refused.
const maxInputRows = 5

// contentRows is how much text the box will hold, and is as good as no limit.
//
// A limit has to be given: without one the widget reads maxInputRows as a
// limit on the text as well as on the box, and stops taking lines once the
// visible ones are used up — which would let the height of a band on screen
// decide what a document may contain.
//
// It is counted in rows as they would be drawn, so any number small enough to
// reach is a number a long enough value reaches, and what the widget does then
// is take none of it at all: a value that arrived whole would be held as
// nothing, and Enter would commit the nothing. The answer is a limit no value
// can reach rather than one chosen to be generous.
const contentRows = math.MaxInt32

// editor is the box an answer is typed into.
//
// It wraps the two widgets so that everything above sees one thing. Which of
// them is in use follows from whether the answer may hold a newline, which the
// prompt has already decided (application.PromptInfo.Multiline): a widget that
// cannot make a newline is how "no newline belongs here" is enforced.
//
// It is a value, copied with the model it sits in, as the widgets themselves
// are meant to be used.
//
// The zero value is no box at all, which is what the model holds while nothing
// is being typed into: every method below answers for it without touching a
// widget that was never built. That is what lets the model drop a box by
// forgetting it, rather than by remembering that it has none.
type editor struct {
	live      bool
	multiline bool

	input textinput.Model
	area  textarea.Model
}

// newEditor is a box holding what the session asked to have edited, and
// whether it holds it.
//
// The widget's own decorations are turned off rather than restyled: a prompt
// character, line numbers and a highlighted current line all belong to a form
// on a page of its own, and this is one band under a document. What is left is
// the text and the caret.
//
// It answers false when the box did not take the value whole. Nothing should
// make that happen — the limits below are past anything a terminal can draw —
// but a box holding less than it was given is the one failure that must not
// pass silently: Enter would commit a value nobody typed.
func newEditor(t Theme, seed application.EffectBeginInput, width int) (editor, bool) {
	e := editor{live: true, multiline: seed.Multiline}

	if !e.multiline {
		e.input = textinput.New()
		e.input.Prompt = ""

		// Before Focus, which reads the cursor's mode out of them.
		e.input.SetStyles(inputStyles(t))
		e.input.SetWidth(max(width, 1))
		e.input.SetValue(seed.Text)
		e.input.CursorEnd()
		e.input.Focus()

		return e, e.Value() == seed.Text
	}

	e.area = textarea.New()
	e.area.Prompt = ""
	e.area.ShowLineNumbers = false
	e.area.SetStyles(areaStyles(t))

	// The box is as tall as what is in it, up to the share of the screen it is
	// allowed, and never shorter than a row.
	e.area.DynamicHeight = true
	e.area.MinHeight = 1
	e.area.MaxHeight = maxInputRows
	e.area.MaxContentHeight = contentRows

	// After the prompt and the line numbers, which the width is measured
	// around.
	e.area.SetWidth(max(width, 1))
	e.area.SetValue(seed.Text)

	// The widget will not hold more lines than a limit of its own, and it takes
	// what it was given as far as that limit and no further. A value cut there
	// would be committed short by someone who only looked at it, so the one
	// spelling that cannot be cut that way is used instead: a single line,
	// however many breaks the value has. Both spellings read back the same.
	held := e.area.Value() == seed.Text
	if !held {
		e.area.SetValue(seed.OneLine)
		held = e.area.Value() == seed.OneLine
	}

	e.area.CursorEnd()
	e.area.Focus()

	return e, held
}

// Value is what has been typed.
func (e editor) Value() string {
	switch {
	case !e.live:
		return ""

	case e.multiline:
		return e.area.Value()
	}

	return e.input.Value()
}

// Rows is how many rows the box takes on screen, and none when there is no box.
func (e editor) Rows() int {
	switch {
	case !e.live:
		return 0

	case e.multiline:
		return max(e.area.Height(), 1)
	}

	return 1
}

// Update hands a message to the widget: a key press, or a paste.
//
// The keys that end an edit never reach here: Enter, Esc and Ctrl+j are the
// prompt's rather than the box's, and are taken before this is called.
//
// The command the widget answers with is dropped. Two things produce one, and
// neither is wanted: a blinking caret, which is turned off below, and Ctrl+V,
// which asks the operating system for its clipboard. pino does not reach for
// that (DESIGN §17) — many of its readers are on the far end of an ssh
// connection, where the clipboard it would read is the wrong machine's. What
// the terminal itself pastes arrives as a message and is taken here.
func (e editor) Update(msg tea.Msg) editor {
	switch {
	case !e.live:
		return e

	case e.multiline:
		e.area, _ = e.area.Update(msg)

		return e
	}

	e.input, _ = e.input.Update(msg)

	return e
}

// InsertNewline breaks the line at the caret, which is what Ctrl+j asks for.
//
// Nothing happens in a box that cannot hold one. A prompt that offers Ctrl+j
// is a prompt for a string, and only a string may contain a newline.
func (e editor) InsertNewline() editor {
	if e.live && e.multiline {
		e.area.InsertString("\n")
	}

	return e
}

// SetWidth fits the box to the room the band has for it.
func (e editor) SetWidth(width int) editor {
	switch {
	case !e.live:
		return e

	case e.multiline:
		e.area.SetWidth(max(width, 1))

		return e
	}

	e.input.SetWidth(max(width, 1))

	return e
}

// View is the box as rows, the caret drawn in place.
//
// Rows rather than one string, because the band lays them out beside a title
// and a hint: what the widget produces is content, and where it goes on the
// screen is not its to decide.
func (e editor) View() []string {
	switch {
	case !e.live:
		return nil

	case e.multiline:
		return strings.Split(e.area.View(), "\n")
	}

	return []string{e.input.View()}
}

// inputStyles and areaStyles dress the widgets in the theme's own.
//
// The caret takes the colour of the text around it, so that the band is drawn
// in one palette without the theme holding a field for a cursor nobody else
// can use. Blinking is turned off: a blinking caret is driven by a timer, and
// answering it would mean carrying messages that are not key presses through a
// layer whose whole job is to turn key presses into Actions.
func inputStyles(t Theme) textinput.Styles {
	s := textinput.DefaultDarkStyles()

	s.Focused.Text = t.Prompt
	s.Focused.Prompt = t.Prompt
	s.Focused.Placeholder = t.PromptHint
	s.Cursor.Color = t.Prompt.GetForeground()
	s.Cursor.Blink = false

	return s
}

func areaStyles(t Theme) textarea.Styles {
	s := textarea.DefaultDarkStyles()

	for _, state := range []*textarea.StyleState{&s.Focused, &s.Blurred} {
		state.Base = t.Prompt
		state.Text = t.Prompt
		state.Prompt = t.Prompt
		state.Placeholder = t.PromptHint

		// The band is a few rows under a document, so the row the caret is on
		// is not marked: there is nothing here to lose it among.
		state.CursorLine = t.Prompt
		state.CursorLineNumber = t.Prompt
		state.LineNumber = t.Prompt
		state.EndOfBuffer = t.Prompt
	}

	s.Cursor.Color = t.Prompt.GetForeground()
	s.Cursor.Blink = false

	return s
}
