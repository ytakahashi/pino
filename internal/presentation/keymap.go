package presentation

import (
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

// String is the prefix as it was typed, which is what the status bar shows.
// Nothing waiting is nothing to show.
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

// Resolve is the Action a key press stands for, along with the prefix left
// waiting afterwards. Either may be empty: a prefix produces no Action, and
// most keys leave nothing waiting.
//
// The table lives here and the meaning of an Action lives in the application
// layer, which splits the interaction along a line testable from both sides:
// a key table on one side, a state transition on the other. Nothing in this
// function knows what quitting does.
//
// The prefix is a parameter and a result rather than a field of something,
// which keeps this a plain function of what was typed and what was waiting.
// A model that forgot to store the result would otherwise sit waiting for a
// second key for the rest of the session.
//
// It takes a key press rather than the tea.KeyMsg interface, which also
// covers releases: a terminal that reports both would otherwise resolve one
// keystroke to the same Action twice.
func Resolve(k tea.KeyPressMsg, mode application.Mode, pending Pending) (application.Action, Pending) {
	// The terminal's own way out is bound before anything else is consulted,
	// so that no mode and no half-typed sequence can become a dead end. A mode
	// that wants Ctrl+C for something else has to claim it here, rather than
	// getting it by being the one the key press happens to reach.
	if k.String() == "ctrl+c" {
		return application.ActionQuit{}, PendingNone
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
			return resolvePending(k, pending), PendingNone
		}

		return resolveNormal(k)

	case application.ModeEdit, application.ModeInsert, application.ModeConfirm, application.ModeHelp:
		return nil, PendingNone
	}

	return nil, PendingNone
}

// resolveNormal is the table for reading a document.
func resolveNormal(k tea.KeyPressMsg) (application.Action, Pending) {
	// Matching the textual form of the keystroke rather than the key code and
	// its modifiers keeps the table readable and, more importantly, keeps q
	// and Q apart: vim gives shifted keys their own meanings, and pino
	// follows.
	switch k.String() {
	case "j", "down":
		return application.ActionMoveNext{}, PendingNone
	case "k", "up":
		return application.ActionMovePrev{}, PendingNone
	case "h", "left":
		return application.ActionMoveOut{}, PendingNone
	case "l", "right":
		return application.ActionMoveIn{}, PendingNone

	// Tab alone. A terminal reports it as its own key rather than as the
	// character it once was, so "shift+tab" falls through to nothing: there
	// are two views and one key between them, and stepping backwards through
	// two is stepping forwards.
	case "tab":
		return application.ActionToggleView{}, PendingNone

	case "G":
		return application.ActionMoveLast{}, PendingNone
	case "ctrl+d":
		return application.ActionScrollHalfDown{}, PendingNone
	case "ctrl+u":
		return application.ActionScrollHalfUp{}, PendingNone

	// Editing. What Enter does depends on what is selected, which is why one
	// key covers six answers: this table says that the document is to be acted
	// on, and the layer holding it says what acting on it means.
	case "enter":
		return application.ActionEdit{}, PendingNone
	case "r":
		return application.ActionRenameKey{}, PendingNone
	case "t":
		return application.ActionChangeType{}, PendingNone

	case "u":
		return application.ActionUndo{}, PendingNone
	case "ctrl+r":
		return application.ActionRedo{}, PendingNone

	// Nothing is bound to g or z alone, so there is no ambiguity to time out
	// of: the next key press decides, however long it takes to arrive.
	case "g":
		return nil, PendingG
	case "z":
		return nil, PendingZ

	case "q":
		return application.ActionQuit{}, PendingNone
	}

	// Esc is among the keys that fall through here. In normal mode it means
	// nothing, and cancelling a half-typed sequence is what falling through
	// already does.
	return nil, PendingNone
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

// resolvePending is the key that completes a prefix.
//
// A key the prefix has no meaning for cancels it and does nothing else. It is
// not carried out on its own: gj would then move down, which is a typing slip
// turned into a movement.
func resolvePending(k tea.KeyPressMsg, pending Pending) application.Action {
	switch pending {
	case PendingG:
		if k.String() == "g" {
			return application.ActionMoveFirst{}
		}

	case PendingZ:
		switch k.String() {
		case "R":
			return application.ActionExpandAll{}
		case "M":
			return application.ActionCollapseAll{}
		}

	case PendingNone:
		// Not reached: the caller resolves against the table instead.
	}

	return nil
}
