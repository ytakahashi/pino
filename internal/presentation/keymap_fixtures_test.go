package presentation

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application"
)

// allModes is every mode the application defines. Only normal is reachable
// so far; the rest are listed so that a binding said to work everywhere is
// checked everywhere rather than only where it happens to be used today.
var allModes = []application.Mode{
	application.ModeNormal,
	application.ModeEdit,
	application.ModeInsert,
	application.ModeConfirm,
	application.ModeHelp,
}

func key(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func shifted(base, upper rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: base, Mod: tea.ModShift, ShiftedCode: upper, Text: string(upper)}
}

func ctrl(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }

func special(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }

// feed types a sequence of keys, answering the Actions it produced and the
// prefix left waiting at the end.
func feed(mode application.Mode, keys ...tea.KeyPressMsg) ([]application.Action, Pending) {
	var (
		got     []application.Action
		pending Pending
	)

	for _, k := range keys {
		var act application.Action

		act, pending = Resolve(k, mode, pending)
		if act != nil {
			got = append(got, act)
		}
	}

	return got, pending
}

func assertOneAction(t *testing.T, got []application.Action, want application.Action) {
	t.Helper()

	if len(got) != 1 {
		t.Fatalf("produced %v, want exactly %v", got, want)
	}

	if got[0] != want {
		t.Errorf("produced %v, want %v", got[0], want)
	}
}
