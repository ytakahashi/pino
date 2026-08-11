// Package presentation draws pino and turns key presses into Actions.
//
// It owns the terminal and nothing else: what is shown comes from the
// application layer as lines carrying a Role per span, and what a key press
// means is decided there too. The two decisions made here are which colour a
// Role gets and which key maps to which Action. Both are display concerns,
// and keeping them here is what allows rendering to live below without a
// terminal library reaching into it.
package presentation

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ytakahashi/pino/internal/application"
)

// Theme is how each Role is drawn.
//
// The styles sit in named fields rather than in a map keyed by Role so that
// style below can be written as a switch: a Role added later, a comment once
// JSONC is supported, then fails the exhaustive linter instead of quietly
// rendering with a zero style. Holding them in a value rather than a package
// table also keeps the colours injectable, which is what an option to choose
// a theme would need.
type Theme struct {
	Key         lipgloss.Style
	StringValue lipgloss.Style
	NumberValue lipgloss.Style
	BoolValue   lipgloss.Style
	NullValue   lipgloss.Style
	Punct       lipgloss.Style
	TreeGuide   lipgloss.Style

	// StatusBar is the strip along the bottom of the screen. It is not a
	// Role: no renderer produces it, and it is drawn around the document
	// rather than as part of it.
	StatusBar lipgloss.Style

	// Rule divides the document from the inspector, and FieldName is what the
	// inspector calls each of the things it says. Neither is a Role, for the
	// reason StatusBar is not: no renderer produces them, and they belong to
	// how the screen is divided rather than to what the document holds.
	Rule      lipgloss.Style
	FieldName lipgloss.Style

	// Cursor is laid over the row the selection is on, keeping each span's own
	// colour: it says which row, not what is in it.
	//
	// Whatever it sets has to be something the spans leave unset, since a span
	// that has made its own choice keeps it. In practice that means a
	// background, which is why no Role has one.
	Cursor lipgloss.Style
}

// DefaultTheme is what pino draws with when nothing else is chosen.
//
// The colours are picked from the 256 colour cube rather than from the
// sixteen basic ones, so that a document looks the same wherever it is opened
// instead of following whatever the terminal's palette has been set to.
// Bubble Tea detects the terminal's colour profile when the program starts
// and downsamples what is written, so a terminal that cannot do 256 colours
// still gets the nearest it has.
//
// Each of the value kinds gets a colour of its own: telling a number from a
// string that happens to hold digits is the reason for colouring the document
// at all. Punctuation is dimmer than every value, so that the structure
// stays readable without competing with the contents.
func DefaultTheme() Theme {
	return Theme{
		Key:         lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
		StringValue: lipgloss.NewStyle().Foreground(lipgloss.Color("113")),
		NumberValue: lipgloss.NewStyle().Foreground(lipgloss.Color("215")),
		BoolValue:   lipgloss.NewStyle().Foreground(lipgloss.Color("170")),
		NullValue:   lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Punct:       lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		TreeGuide:   lipgloss.NewStyle().Foreground(lipgloss.Color("240")),

		// The bar is set apart by a filled background rather than by a rule
		// above it, which would cost one of the rows the document is drawn in.
		StatusBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("238")),

		// The rule divides without being read, so it is the dimmest thing on
		// the screen. The names in the inspector are read but are not the
		// answer to anything, so they are dimmer than what stands beside them
		// and brighter than the rule.
		Rule:      lipgloss.NewStyle().Foreground(lipgloss.Color("238")),
		FieldName: lipgloss.NewStyle().Foreground(lipgloss.Color("245")),

		// The selected row is marked by a band behind it rather than by an
		// arrow in front of it: an arrow would need a column of its own and
		// push the whole document sideways, and reading JSON in its usual
		// shape is what the view is for. The grey is dark enough to leave
		// every value colour legible and distinct from the bar's.
		Cursor: lipgloss.NewStyle().Background(lipgloss.Color("237")),
	}
}

// style is how text in role r is drawn.
//
// An unknown role is drawn without styling rather than reported: a role this
// theme does not know is a gap in the theme, and losing a colour is a better
// outcome for the person reading the document than losing the text.
func (t Theme) style(r application.Role) lipgloss.Style {
	switch r {
	case application.RoleKey:
		return t.Key
	case application.RoleStringValue:
		return t.StringValue
	case application.RoleNumberValue:
		return t.NumberValue
	case application.RoleBoolValue:
		return t.BoolValue
	case application.RoleNullValue:
		return t.NullValue
	case application.RolePunct:
		return t.Punct
	case application.RoleTreeGuide:
		return t.TreeGuide
	}

	return lipgloss.NewStyle()
}

// RenderLine draws one row of a document, leading indentation included.
//
// The spans of a line hold its content only, so the indentation is built here
// from the depth of the row. That is what lets one line render as whitespace
// in the JSON view and as guides in the tree view, and it is why the width of
// a level is not the line's to decide.
//
// indent is one level of indentation in the view being drawn, and is a
// parameter rather than a property of the theme: the JSON view draws with the
// document's own, since that whitespace is what will be written back, while
// the tree view draws with a width of its own choosing because nothing it
// shows is ever saved. Which of the two applies is settled by the caller, in
// indentFor. On a row that is not selected the indentation is left unstyled,
// since styling whitespace only emits escape sequences around nothing.
//
// selected marks the row the cursor is on. The cursor's styling is laid over
// each span in turn rather than around the row as a whole: a style wrapping
// the finished row would end at the first span that reset its own colours,
// leaving the band broken wherever the document is at its most colourful.
func (t Theme) RenderLine(l application.Line, indent string, selected bool) string {
	var b strings.Builder

	if leading := strings.Repeat(indent, l.Depth); selected {
		b.WriteString(t.Cursor.Render(leading))
	} else {
		b.WriteString(leading)
	}

	for _, s := range l.Spans {
		b.WriteString(t.decorate(t.style(s.Role), selected).Render(s.Text))
	}

	return b.String()
}

// decorate lays the cursor over a span's own styling on the selected row.
//
// Inherit takes only what the span has not settled for itself, which is what
// keeps each value its own colour while the row gains a background.
func (t Theme) decorate(s lipgloss.Style, selected bool) lipgloss.Style {
	if !selected {
		return s
	}

	return s.Inherit(t.Cursor)
}

// RenderCursorFill is the remainder of the selected row: width columns of the
// cursor's styling and nothing else.
//
// The band has to reach the edge of the screen. Stopping where the text does
// would make how far it reached depend on how long each row happened to be,
// which reads as ragged rather than as a row being pointed at.
func (t Theme) RenderCursorFill(width int) string {
	if width <= 0 {
		return ""
	}

	return t.Cursor.Render(strings.Repeat(" ", width))
}
