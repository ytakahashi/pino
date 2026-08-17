package presentation

import (
	"strings"
)

// The help screen is the key tables read back to whoever is using them. It is
// built from those tables rather than written out beside them, so a key that
// changes there changes here, and it is one page rather than something to
// scroll: the whole of pino's keyboard fits in the rows the smallest terminal
// pino draws in has, and a screen with an offset would be a second place a key
// press could mean something.

// helpGroup is the heading an entry appears under.
//
// The zero value is no heading at all, so a binding that was given none is
// left off the screen rather than quietly gathered under the first one — which
// is what lets a test say the screen holds every binding exactly once.
type helpGroup uint8

const (
	helpNone helpGroup = iota
	helpMove
	helpJump
	helpFold
	helpView
	helpEdit
	helpStructure
	helpHistory
	helpPrompt
)

// helpGroups is the order the headings are read in: getting about the
// document, then looking at it, then changing it, then what becomes of the
// changes, and last the keys that exist only while a question is up.
var helpGroups = []helpGroup{
	helpMove,
	helpJump,
	helpFold,
	helpView,
	helpEdit,
	helpStructure,
	helpHistory,
	helpPrompt,
}

// String is the heading as it is written down the left of the screen.
func (g helpGroup) String() string {
	switch g {
	case helpMove:
		return "Move"
	case helpJump:
		return "Jump"
	case helpFold:
		return "Fold"
	case helpView:
		return "View"
	case helpEdit:
		return "Edit"
	case helpStructure:
		return "Structure"
	case helpHistory:
		return "History"
	case helpPrompt:
		return "Prompt"
	case helpNone:
		return ""
	}

	return ""
}

// helpEntry is one key and what it does, as the screen puts it.
type helpEntry struct {
	Group       helpGroup
	Keys        string
	Description string
}

// helpLine is one heading and everything written beside it.
//
// The screen is composed into these once and drawn from them twice, plainly
// and in colour, so that what a test measures is what a terminal shows.
type helpLine struct {
	Heading string
	Entries []helpEntry
}

// contextualEntries are the keys that belong to something on screen rather
// than to the document.
//
// They are written here and not in the key tables because they are not in
// them: what Enter does to a prompt is decided by the prompt, and the wheel is
// not a key at all. Leaving them off would be worse than holding them in a
// second place — a reader who cannot get out of a text box is stuck — so what
// the tables cannot say is said here, beside what they can.
var contextualEntries = []helpEntry{
	{Group: helpView, Keys: "wheel", Description: "scroll"},
	{Group: helpView, Keys: "--no-mouse", Description: "select"},
	{Group: helpPrompt, Keys: "Enter", Description: "accept"},
	{Group: helpPrompt, Keys: "Esc", Description: "cancel"},
	{Group: helpPrompt, Keys: "Ctrl+j", Description: "newline"},
}

// helpTitle is what the screen calls itself, at the left of its first row.
const helpTitle = "pino help"

// helpLabelWidth is the column the entries begin in: the longest heading and a
// space. It is fixed rather than fitted to the headings on screen, so that the
// keys line up under one another however the tables come to be grouped.
const helpLabelWidth = len("Structure") + 1

// helpEntryGap divides one entry from the next. Two spaces, since a single one
// would read as though the keys of the next entry belonged to this one's
// description.
const helpEntryGap = "  "

// helpEntries is every entry the screen holds, in the order it holds them.
//
// The sequences come before the single keys within a heading, which is what
// puts "gg first" beside "G last" in the order a reader would say them.
func helpEntries() []helpEntry {
	entries := make([]helpEntry, 0, len(pendingBindings)+len(normalBindings)+len(contextualEntries))

	for _, b := range pendingBindings {
		entries = append(entries, helpEntry{Group: b.Group, Keys: b.HelpKeys, Description: b.Description})
	}

	for _, b := range normalBindings {
		entries = append(entries, helpEntry{Group: b.Group, Keys: b.HelpKeys, Description: b.Description})
	}

	return append(entries, contextualEntries...)
}

// helpLines is the body of the screen: one line per heading, in order.
//
// A heading with nothing under it is left out rather than drawn empty, so the
// screen is as short as the tables make it. What it must not do is grow past
// the rows the smallest terminal has, which is what the test alongside holds
// it to; there is no offset here to scroll a longer one with.
func helpLines() []helpLine {
	entries := helpEntries()

	lines := make([]helpLine, 0, len(helpGroups))

	for _, g := range helpGroups {
		var under []helpEntry

		for _, e := range entries {
			if e.Group == g {
				under = append(under, e)
			}
		}

		if len(under) == 0 {
			continue
		}

		lines = append(lines, helpLine{Heading: g.String(), Entries: under})
	}

	return lines
}

// helpRows is the screen as plain text: the title row, then the body.
//
// width is where the title row's two ends are spread to. The body does not use
// it: those rows are as long as they are, and a terminal too narrow for them is
// one pino has already refused to draw in.
func helpRows(width int) []string {
	rows := []string{helpTitleRow(width)}

	for _, l := range helpLines() {
		written := make([]string, 0, len(l.Entries))
		for _, e := range l.Entries {
			written = append(written, e.Keys+" "+e.Description)
		}

		rows = append(rows, pad(l.Heading, helpLabelWidth)+strings.Join(written, helpEntryGap))
	}

	return rows
}

// helpTitleRow is what the screen calls itself and how to leave it, at the two
// ends of one row.
//
// The ways out are built from the keys the screen actually takes, so the offer
// and its keeping are one thing — the same reason a prompt draws the keys it
// will answer to. They are spread to the far side the way the status bar's are,
// where a reader is already used to finding what is true of the whole screen.
func helpTitleRow(width int) string {
	return spread(helpTitle, strings.Join(keyLabels(helpClose), " / ")+" close", width)
}

// RenderHelp draws the help screen: exactly height rows of exactly width
// columns.
//
// It is a block for the reason the inspector is one. The screen it is drawn on
// holds a status bar at the bottom with whatever comes before it, so a row too
// many would push the bar off and a row too few would let what is underneath
// show through.
//
// The heading and the keys are styled apart from the words about them, which is
// what lets the eye find a key without reading the rows. The three columns are
// already three fields, so nothing has to be taken apart again to colour it.
func (t Theme) RenderHelp(width, height int) []string {
	rows := []string{t.HelpTitle.Render(helpTitleRow(width))}

	for _, l := range helpLines() {
		written := make([]string, 0, len(l.Entries))
		for _, e := range l.Entries {
			written = append(written, t.HelpKey.Render(e.Keys)+" "+e.Description)
		}

		rows = append(rows,
			t.HelpGroup.Render(pad(l.Heading, helpLabelWidth))+strings.Join(written, helpEntryGap))
	}

	return fitRows(rows, width, height)
}
