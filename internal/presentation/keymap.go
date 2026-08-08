package presentation

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application"
)

// Resolve is the Action a key press stands for, or nil when the key is not
// bound in this mode.
//
// The table lives here and the meaning of an Action lives in the application
// layer, which splits the interaction along a line testable from both sides:
// a key table on one side, a state transition on the other. Nothing in this
// function knows what quitting does.
//
// It takes a key press rather than the tea.KeyMsg interface, which also
// covers releases: a terminal that reports both would otherwise resolve one
// keystroke to the same Action twice.
func Resolve(k tea.KeyPressMsg, mode application.Mode) application.Action {
	// The terminal's own way out is bound before the mode is consulted, so
	// that no mode can become a dead end by binding nothing itself. A mode
	// that wants Ctrl+C for something else has to claim it here, rather than
	// getting it by being the one the key press happens to reach.
	if k.String() == "ctrl+c" {
		return application.ActionQuit{}
	}

	// The remaining bindings are per mode because the same key means
	// different things in each: q types a character while editing and leaves
	// pino otherwise. Only normal mode is reachable so far, and the switch
	// covers the whole set so that a mode added later is reported here rather
	// than silently resolving to nothing.
	switch mode {
	case application.ModeNormal:
		return resolveNormal(k)

	case application.ModeEdit, application.ModeInsert, application.ModeConfirm, application.ModeHelp:
		return nil
	}

	return nil
}

// resolveNormal is the table for moving around a document.
func resolveNormal(k tea.KeyPressMsg) application.Action {
	// Matching the textual form of the keystroke rather than the key code and
	// its modifiers keeps the table readable and, more importantly, keeps q
	// and Q apart: vim gives shifted keys their own meanings, and pino
	// follows.
	switch k.String() {
	case "q":
		return application.ActionQuit{}
	}

	return nil
}
