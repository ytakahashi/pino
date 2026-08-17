package presentation

import (
	"slices"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application"
)

// Pending is a key that has been typed and is waiting for the one that
// completes it, as g does in gg and z does in zM.
//
// It lives here rather than in the application because it is about how a
// request was made, not about what was asked for: the application is handed
// ActionMoveFirst and has no idea whether it took one keystroke or two.
type Pending uint8

const (
	PendingNone Pending = iota
	PendingG
	PendingZ
)

// TerminalAction is a request to change how pino uses the terminal rather
// than to change or navigate the document. It stays in presentation because
// the application has no terminal behaviour to carry out.
type TerminalAction uint8

const (
	TerminalNone TerminalAction = iota
	TerminalToggleMouse
)

// String is the prefix as it was typed, which is what the status bar shows.
// Nothing waiting is nothing to show.
//
// It is also the key that starts the prefix, and resolving reads it as such
// rather than listing the same letter a second time.
func (p Pending) String() string {
	switch p {
	case PendingG:
		return "g"
	case PendingZ:
		return "z"
	case PendingNone:
		return ""
	}

	return ""
}

// binding is one row of the key table: every spelling that asks for one
// Action, the Action asked for, and how the row reads on the help screen.
//
// The keys are spelled as a terminal reports them, since that is what the
// table is matched against. Keys[0] is the one written back to a reader, so a
// row answers both of the questions asked of the table: what a press means,
// and which key a request is made by.
//
// A row is one entry of the help screen and never more than one. What that
// costs is a key whose entry would read differently under two headings — Enter
// both folds and edits — and what it buys is a screen that cannot fall out of
// step with the table: every row is on it exactly once, which is a property a
// test can hold the two to.
type binding struct {
	Keys   []string
	Action application.Action

	// Group is the heading the entry appears under, and HelpKeys and
	// Description are the entry itself.
	//
	// The words are here rather than beside the Action they name because they
	// are neither: what ActionMoveNext means belongs to the application, while
	// "j/↓ next" is four words chosen to fit beside seven others in 60
	// columns. HelpKeys is spelled for a reader rather than as a terminal
	// reports the key, which is what lets one entry name an arrow and a letter.
	Group       helpGroup
	HelpKeys    string
	Description string
}

// normalBindings is the table for reading a document.
//
// It is one table rather than one per reader. What a press means and which key
// the inspector advertises an operation as were separate lists of the same
// knowledge, and a key rebound in one of them was a difference only a test
// could find.
//
// Matching the textual form of the keystroke rather than the key code and its
// modifiers keeps the table readable and, more importantly, keeps q and Q
// apart: vim gives shifted keys their own meanings, and pino follows.
var normalBindings = []binding{
	{
		Keys: []string{"j", "down"}, Action: application.ActionMoveNext{},
		Group: helpMove, HelpKeys: "j/↓", Description: "next",
	},
	{
		Keys: []string{"k", "up"}, Action: application.ActionMovePrev{},
		Group: helpMove, HelpKeys: "k/↑", Description: "previous",
	},
	{
		Keys: []string{"h", "left"}, Action: application.ActionMoveOut{},
		Group: helpMove, HelpKeys: "h/←", Description: "out",
	},
	{
		Keys: []string{"l", "right"}, Action: application.ActionMoveIn{},
		Group: helpMove, HelpKeys: "l/→", Description: "in",
	},

	// Tab alone. A terminal reports it as its own key rather than as the
	// character it once was, so "shift+tab" matches no row: there are two
	// views and one key between them, and stepping backwards through two is
	// stepping forwards.
	{
		Keys: []string{"tab"}, Action: application.ActionToggleView{},
		Group: helpView, HelpKeys: "Tab", Description: "view",
	},

	// The key that asks for the screen this row is written on. It is listed
	// there for a reader who arrived some other way and wants to know how they
	// did it; how to leave is on the title row instead, where it is needed.
	{
		Keys: []string{"?"}, Action: application.ActionShowHelp{},
		Group: helpView, HelpKeys: "?", Description: "help",
	},

	{
		Keys: []string{"G"}, Action: application.ActionMoveLast{},
		Group: helpJump, HelpKeys: "G", Description: "last",
	},
	{
		Keys: []string{"ctrl+d"}, Action: application.ActionScrollHalfDown{},
		Group: helpJump, HelpKeys: "Ctrl+d", Description: "down",
	},
	{
		Keys: []string{"ctrl+u"}, Action: application.ActionScrollHalfUp{},
		Group: helpJump, HelpKeys: "Ctrl+u", Description: "up",
	},

	// Editing. What Enter does depends on what is selected, which is why one
	// key covers six answers: this table says that the document is to be acted
	// on, and the layer holding it says what acting on it means. The entry says
	// both of the things a reader will meet, since a row promising only one of
	// them would read as a key that does not work on the other.
	{
		Keys: []string{"enter"}, Action: application.ActionEdit{},
		Group: helpEdit, HelpKeys: "Enter", Description: "value/fold",
	},
	{
		Keys: []string{"r"}, Action: application.ActionRenameKey{},
		Group: helpEdit, HelpKeys: "r", Description: "key",
	},
	{
		Keys: []string{"a"}, Action: application.ActionAddChild{},
		Group: helpStructure, HelpKeys: "a", Description: "child",
	},
	{
		Keys: []string{"A"}, Action: application.ActionAddSibling{},
		Group: helpStructure, HelpKeys: "A", Description: "sibling",
	},
	{
		Keys: []string{"d"}, Action: application.ActionDelete{},
		Group: helpStructure, HelpKeys: "d", Description: "delete",
	},
	{
		Keys: []string{"t"}, Action: application.ActionChangeType{},
		Group: helpEdit, HelpKeys: "t", Description: "type",
	},

	{
		Keys: []string{"u"}, Action: application.ActionUndo{},
		Group: helpHistory, HelpKeys: "u", Description: "undo",
	},
	{
		Keys: []string{"ctrl+r"}, Action: application.ActionRedo{},
		Group: helpHistory, HelpKeys: "Ctrl+r", Description: "redo",
	},

	// Saving is a control key rather than a letter, as it is in the editors
	// pino sits beside on a terminal. What it does when there is nothing to
	// write, or when the file has changed underneath, is not decided here.
	{
		Keys: []string{"ctrl+s"}, Action: application.ActionSave{},
		Group: helpHistory, HelpKeys: "Ctrl+s", Description: "save",
	},

	// Ctrl+C asks for the same thing and is bound above every mode rather than
	// in this table, so it is named on this row instead of getting one of its
	// own: two entries for one request would be the screen saying pino can be
	// left twice.
	{
		Keys: []string{"q"}, Action: application.ActionQuit{},
		Group: helpHistory, HelpKeys: "q/Ctrl+c", Description: "quit",
	},
}

// terminalBinding is one row of the keys that change how pino uses the
// terminal. These keys resolve alongside the document's keys, but their
// requests never cross into the application layer.
type terminalBinding struct {
	Keys     []string
	Terminal TerminalAction

	Group       helpGroup
	HelpKeys    string
	Description string
}

// This table is separate from normalBindings so that every row there keeps
// carrying an application Action. Combining the two would require nil Actions
// and weaken the invariant that every document binding asks for something.
var terminalBindings = []terminalBinding{
	{
		Keys: []string{"m"}, Terminal: TerminalToggleMouse,
		Group: helpView, HelpKeys: "m", Description: "select",
	},
}

// helpClose is every key that puts the help screen away.
//
// Three, because three different habits bring a reader to the same wish: the
// key that opened it, the key that withdraws everything else in pino, and the
// key that leaves a pager. What is drawn on the title row is built from this
// list, so the screen cannot come to advertise a way out it does not take.
var helpClose = []string{"?", "esc", "q"}

// pendingBinding is one row of the table for sequences of two keys: the prefix
// typed first, the keys that complete it, and how the sequence reads on the
// help screen.
type pendingBinding struct {
	Prefix Pending
	Keys   []string
	Action application.Action

	Group       helpGroup
	HelpKeys    string
	Description string
}

// pendingBindings is every sequence a document is read by.
//
// No row carries PendingNone, which is what says that a prefix is the only way
// into this table. The keys that start the prefixes are not written here
// either: they are the prefixes as they are spelled, and resolving reads them
// from there.
var pendingBindings = []pendingBinding{
	{
		Prefix: PendingG, Keys: []string{"g"}, Action: application.ActionMoveFirst{},
		Group: helpJump, HelpKeys: "gg", Description: "first",
	},
	{
		Prefix: PendingZ, Keys: []string{"R"}, Action: application.ActionExpandAll{},
		Group: helpFold, HelpKeys: "zR", Description: "expand all",
	},
	{
		Prefix: PendingZ, Keys: []string{"M"}, Action: application.ActionCollapseAll{},
		Group: helpFold, HelpKeys: "zM", Description: "collapse all",
	},
}

// Resolve is the application or terminal request a key press stands for,
// along with the prefix left waiting afterwards. Each may be empty: a prefix
// produces no request, and most keys leave nothing waiting.
//
// The tables live here and the meaning of an application Action lives in the
// application layer, which splits the interaction along a line testable from
// both sides: a key table on one side, a state transition on the other.
// Terminal requests remain on this side of that boundary. Nothing in this
// function knows what quitting or toggling mouse reporting does.
//
// The prefix is a parameter and a result rather than a field of something,
// which keeps this a plain function of what was typed and what was waiting.
// A model that forgot to store the result would otherwise sit waiting for a
// second key for the rest of the session.
//
// It takes a key press rather than the tea.KeyMsg interface, which also
// covers releases: a terminal that reports both would otherwise resolve one
// keystroke to the same Action twice.
func Resolve(
	k tea.KeyPressMsg,
	mode application.Mode,
	pending Pending,
) (application.Action, TerminalAction, Pending) {
	// The terminal's own way out is bound before anything else is consulted,
	// so that no mode and no half-typed sequence can become a dead end. A mode
	// that wants Ctrl+C for something else has to claim it here, rather than
	// getting it by being the one the key press happens to reach.
	if k.String() == "ctrl+c" {
		return application.ActionQuit{}, TerminalNone, PendingNone
	}

	// The remaining bindings are per mode because the same key means
	// different things in each: q types a character while editing and leaves
	// pino otherwise. Only normal mode is reachable so far, and the switch
	// covers the whole set so that a mode added later is reported here rather
	// than silently resolving to nothing. A prefix does not survive a change
	// of mode, since the key that would complete it means something else.
	switch mode {
	case application.ModeNormal:
		if pending != PendingNone {
			return resolvePending(k, pending), TerminalNone, PendingNone
		}

		return resolveNormal(k)

	case application.ModeHelp:
		return resolveHelp(k), TerminalNone, PendingNone

	case application.ModeEdit, application.ModeInsert, application.ModeConfirm:
		return nil, TerminalNone, PendingNone
	}

	return nil, TerminalNone, PendingNone
}

// resolveNormal is what a key press means while a document is being read.
//
// The tables are consulted before the prefixes, so a key that means something
// on its own can never also start a sequence. Nothing is bound to g or z alone,
// which is why there is no ambiguity to time out of: the next key press
// decides, however long it takes to arrive.
func resolveNormal(k tea.KeyPressMsg) (application.Action, TerminalAction, Pending) {
	s := k.String()

	for _, b := range normalBindings {
		if slices.Contains(b.Keys, s) {
			return b.Action, TerminalNone, PendingNone
		}
	}

	for _, b := range terminalBindings {
		if slices.Contains(b.Keys, s) {
			return nil, b.Terminal, PendingNone
		}
	}

	for _, b := range pendingBindings {
		if s == b.Prefix.String() {
			return nil, TerminalNone, b.Prefix
		}
	}

	// Esc is among the keys that fall through here. In normal mode it means
	// nothing, and cancelling a half-typed sequence is what falling through
	// already does.
	return nil, TerminalNone, PendingNone
}

// resolveHelp is what a key press means while the help screen is up.
//
// Only the ways out are bound. A screen listing what the keys do is read
// rather than acted on, and a key that went on working underneath it would act
// on a document nobody can see.
func resolveHelp(k tea.KeyPressMsg) application.Action {
	if slices.Contains(helpClose, k.String()) {
		return application.ActionCloseHelp{}
	}

	return nil
}

// resolvePending is the key that completes a prefix.
//
// A key the prefix has no meaning for cancels it and does nothing else. It is
// not carried out on its own: gj would then move down, which is a typing slip
// turned into a movement.
func resolvePending(k tea.KeyPressMsg, pending Pending) application.Action {
	for _, b := range pendingBindings {
		if b.Prefix == pending && slices.Contains(b.Keys, k.String()) {
			return b.Action
		}
	}

	return nil
}

// ResolveChoice is the Action a key press stands for while a list of choices
// is on screen.
//
// The choices come from the prompt rather than from a table here, because they
// are drawn on it: "[s] string" is a promise that s does something, and the
// promise and its keeping are then one thing. The line this draws with the
// table above is that a key written on the screen belongs to whatever wrote it,
// while a key a reader has to know already belongs here.
//
// A key that is not on offer does nothing. Esc is, at every step of every
// edit, which is why it is taken before the offered keys are looked at rather
// than being one of them.
func ResolveChoice(k tea.KeyPressMsg, p application.PromptInfo) application.Action {
	if k.String() == "esc" {
		return application.ActionCancel{}
	}

	for _, c := range p.Choices {
		if k.String() == string(c.Key) {
			return application.ActionPromptChoose{Key: c.Key}
		}
	}

	return nil
}

// available is the operations that apply to the selected node.
//
// It is worked out here because the table is here: what a node allows is a
// fact the inspector already carries, while which operations there are at all
// is knowledge this file is the only holder of. What they are asked for by is
// the table's to say, so a key rebound there follows the pane advertising it
// without having been written down twice.
//
// The order is the order the pane lists them in, and is chosen here rather
// than taken from the table, so that the table stays free to be ordered for
// whoever else reads it.
//
// What is listed is the operations something happens on, not the ones that
// mean something. The two part company where an operation is carried out to no
// effect, which the layer below cannot tell a reader apart from one it refused:
// nothing happens either way. A row promising an operation that leaves the
// screen as it was would be the same disappointment as one promising an
// operation that is turned down, so both are left off.
func available(info application.InspectorInfo) []application.Action {
	if info.Type == "" {
		return nil
	}

	var acts []application.Action

	// Enter acts on whatever is selected, except on a container with no fold
	// to toggle: the root is drawn open and has no folded form, and an empty
	// container is drawn as neither open nor folded. A scalar at the root is
	// not one of these — it is typed over like any other — so what is asked
	// first is whether this is a container at all.
	if !info.Container || (info.Naming != application.NamedNone && info.Children > 0) {
		acts = append(acts, application.ActionEdit{})
	}

	acts = append(acts, application.ActionChangeType{})

	if info.Container {
		acts = append(acts, application.ActionAddChild{})
	}
	if info.Naming != application.NamedNone {
		acts = append(acts, application.ActionAddSibling{}, application.ActionDelete{})
	}
	if info.Naming == application.NamedKey {
		acts = append(acts, application.ActionRenameKey{})
	}

	return acts
}

// canonicalKeys is the key each operation is asked for by, in the order given.
//
// An operation no key asks for is left out rather than advertised as something
// unpressable. Nothing on offer is in that position, and a test walking every
// shape a document takes says so.
func canonicalKeys(acts []application.Action) []string {
	keys := make([]string, 0, len(acts))

	for _, act := range acts {
		if k, ok := canonicalKey(act); ok {
			keys = append(keys, k)
		}
	}

	return keys
}

// canonicalKey is the key a request is made by: the first of the spellings the
// table gives it.
//
// The table is searched by equality, so only an Action carrying no argument
// can be found: a row holding ActionScrollBy{Rows: 1} would answer for that
// one distance and deny every other. Nothing named on screen carries an
// argument, and what a wheel produces is not named on screen at all.
func canonicalKey(act application.Action) (string, bool) {
	for _, b := range normalBindings {
		if b.Action == act {
			return b.Keys[0], true
		}
	}

	return "", false
}

// keyLabels is keys as a reader sees them written.
//
// A terminal names a key that has no character of its own in lowercase: enter,
// tab, esc. That is a name rather than a keystroke, and a name printed beside
// an operation is capitalised. A key that is a character is left exactly as it
// stands, since a and A ask for different things and letter case is the whole
// of the difference between them.
func keyLabels(keys []string) []string {
	labels := make([]string, len(keys))

	for i, k := range keys {
		runes := []rune(k)

		if len(runes) == 1 {
			labels[i] = k

			continue
		}

		labels[i] = string(unicode.ToUpper(runes[0])) + string(runes[1:])
	}

	return labels
}
