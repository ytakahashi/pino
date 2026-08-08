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
// indent is one level of indentation of the open document, not a property of
// the theme: it is what the file already uses and what will be written back,
// which is why the status bar reports the same value. It is left unstyled,
// since styling whitespace only emits escape sequences around nothing.
func (t Theme) RenderLine(l application.Line, indent string) string {
	var b strings.Builder

	b.WriteString(strings.Repeat(indent, l.Depth))

	for _, s := range l.Spans {
		b.WriteString(t.style(s.Role).Render(s.Text))
	}

	return b.String()
}
