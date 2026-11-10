package presentation

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application/picker"
)

// pickerBinding describes both a request and the help entry advertising it.
type pickerBinding struct {
	Keys        []string
	Action      picker.Action
	Group       helpGroup
	HelpKeys    string
	Description string
}

var pickerBindings = []pickerBinding{
	{[]string{"j", "down"}, picker.ActionMoveNext{}, helpMove, "j/↓", "next"},
	{[]string{"k", "up"}, picker.ActionMovePrev{}, helpMove, "k/↑", "previous"},
	{[]string{"G"}, picker.ActionMoveLast{}, helpJump, "G", "last"},
	{[]string{"ctrl+d"}, picker.ActionScrollHalfDown{}, helpJump, "Ctrl+d", "down"},
	{[]string{"ctrl+u"}, picker.ActionScrollHalfUp{}, helpJump, "Ctrl+u", "up"},
	{[]string{"/"}, picker.ActionBeginFilter{}, helpFilter, "/", "filter"},
	{[]string{"esc"}, picker.ActionClearFilter{}, helpFilter, "Esc", "clear"},
	{[]string{"enter"}, picker.ActionOpen{}, helpFile, "Enter", "open"},
	// Ctrl+C is resolved before the table, as in the editor, and advertised
	// beside q in the same way the editor's help does.
	{[]string{"q"}, picker.ActionQuit{}, helpFile, "q/Ctrl+c", "quit"},
	{[]string{"?"}, picker.ActionShowHelp{}, helpView, "?", "help"},
}

var pickerFirstBinding = pickerBinding{
	Keys: []string{"g"}, Action: picker.ActionMoveFirst{},
	Group: helpJump, HelpKeys: "gg", Description: "first",
}

// resolvePicker leaves the filter box to the picker model. Ctrl+C is taken
// first so that neither a prefix nor a text box can swallow the way out.
func resolvePicker(k tea.KeyPressMsg, mode picker.Mode, pending Pending) (picker.Action, TerminalAction, Pending) {
	s := k.String()
	if s == "ctrl+c" {
		return picker.ActionQuit{}, TerminalNone, PendingNone
	}

	switch mode {
	case picker.ModeNormal:
		if pending != PendingNone {
			if pending == PendingG && slices.Contains(pickerFirstBinding.Keys, s) {
				return pickerFirstBinding.Action, TerminalNone, PendingNone
			}

			return nil, TerminalNone, PendingNone
		}

		for _, b := range pickerBindings {
			if slices.Contains(b.Keys, s) {
				return b.Action, TerminalNone, PendingNone
			}
		}

		for _, b := range terminalBindings {
			if slices.Contains(b.Keys, s) {
				return nil, b.Terminal, PendingNone
			}
		}

		if s == PendingG.String() {
			return nil, TerminalNone, PendingG
		}

	case picker.ModeHelp:
		if slices.Contains(helpClose, s) {
			return picker.ActionCloseHelp{}, TerminalNone, PendingNone
		}

	case picker.ModeFilter:
		// Every key but Ctrl+C belongs to the filter box, which the picker
		// model resolves along with the box's own Enter and Esc.
	}

	return nil, TerminalNone, PendingNone
}

func pickerHelpLines() []helpLine {
	entries := []helpEntry{{Group: pickerFirstBinding.Group, Keys: pickerFirstBinding.HelpKeys, Description: pickerFirstBinding.Description}}
	for _, b := range pickerBindings {
		entries = append(entries, helpEntry{Group: b.Group, Keys: b.HelpKeys, Description: b.Description})
	}

	entries = append(entries, sharedContextualEntries...)
	for _, b := range terminalBindings {
		entries = append(entries, helpEntry{Group: b.Group, Keys: b.HelpKeys, Description: b.Description})
	}

	return groupedHelp(entries)
}

func pickerHelpTitle(width int) string {
	return spread("pino file picker", strings.Join(keyLabels(helpClose), " / ")+" close", width)
}
