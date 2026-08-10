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

// The keystrokes below are built the way a terminal reports them, since the
// table is matched against their textual form and that is what produces it.

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

func TestResolveInNormalMode(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
		want application.Action
	}{
		{name: "j moves on", key: key('j'), want: application.ActionMoveNext{}},
		{name: "k moves back", key: key('k'), want: application.ActionMovePrev{}},
		{name: "h moves out", key: key('h'), want: application.ActionMoveOut{}},
		{name: "l moves in", key: key('l'), want: application.ActionMoveIn{}},

		// The arrows say the same things, for anyone not reaching for vim.
		{name: "down", key: special(tea.KeyDown), want: application.ActionMoveNext{}},
		{name: "up", key: special(tea.KeyUp), want: application.ActionMovePrev{}},
		{name: "left", key: special(tea.KeyLeft), want: application.ActionMoveOut{}},
		{name: "right", key: special(tea.KeyRight), want: application.ActionMoveIn{}},

		{name: "tab switches views", key: special(tea.KeyTab), want: application.ActionToggleView{}},
		{
			// There are two views and one key between them, so stepping back
			// through them is stepping forward.
			name: "shifted tab is not bound",
			key:  tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift},
			want: nil,
		},

		{name: "G goes to the end", key: shifted('g', 'G'), want: application.ActionMoveLast{}},
		{name: "ctrl+d reads on", key: ctrl('d'), want: application.ActionScrollHalfDown{}},
		{name: "ctrl+u reads back", key: ctrl('u'), want: application.ActionScrollHalfUp{}},

		{name: "q quits", key: key('q'), want: application.ActionQuit{}},
		{
			// Shifted keys carry their own meanings in vim, so Q is not q
			// with a modifier that can be ignored.
			name: "shifted q is not bound",
			key:  shifted('q', 'Q'),
			want: nil,
		},
		{name: "unbound letter", key: key('x'), want: nil},
		{name: "unbound special key", key: special(tea.KeyEscape), want: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, pending := Resolve(tc.key, application.ModeNormal, PendingNone)

			if got != tc.want {
				t.Errorf("Resolve(%q) = %v, want %v", tc.key.String(), got, tc.want)
			}

			if pending != PendingNone {
				t.Errorf("Resolve(%q) left %v waiting, want nothing", tc.key.String(), pending)
			}
		})
	}
}

// A prefix does nothing by itself and waits for what completes it.
func TestResolveStartsAPrefix(t *testing.T) {
	tests := map[string]struct {
		key  tea.KeyPressMsg
		want Pending
	}{
		"g": {key: key('g'), want: PendingG},
		"z": {key: key('z'), want: PendingZ},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, pending := Resolve(tc.key, application.ModeNormal, PendingNone)

			if got != nil {
				t.Errorf("Resolve(%q) = %v, want nothing until the next key", name, got)
			}

			if pending != tc.want {
				t.Errorf("Resolve(%q) left %v waiting, want %v", name, pending, tc.want)
			}
		})
	}
}

func TestResolveCompletesAPrefix(t *testing.T) {
	tests := map[string]struct {
		keys []tea.KeyPressMsg
		want application.Action
	}{
		"gg": {keys: []tea.KeyPressMsg{key('g'), key('g')}, want: application.ActionMoveFirst{}},
		"zR": {keys: []tea.KeyPressMsg{key('z'), shifted('r', 'R')}, want: application.ActionExpandAll{}},
		"zM": {keys: []tea.KeyPressMsg{key('z'), shifted('m', 'M')}, want: application.ActionCollapseAll{}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, pending := feed(application.ModeNormal, tc.keys...)

			assertOneAction(t, got, tc.want)

			if pending != PendingNone {
				t.Errorf("%q left %v waiting, want nothing", name, pending)
			}
		})
	}
}

// A key the prefix knows nothing about cancels it and does nothing else. If it
// were carried out on its own, gj would move down and a typing slip would have
// become a movement.
func TestResolveCancelsAPrefix(t *testing.T) {
	tests := map[string][]tea.KeyPressMsg{
		"g then a bound key":    {key('g'), key('j')},
		"g then an unbound one": {key('g'), key('x')},
		"g then escape":         {key('g'), special(tea.KeyEscape)},
		"g then tab":            {key('g'), special(tea.KeyTab)},
		"z then a bound key":    {key('z'), key('l')},
		"z then the wrong case": {key('z'), key('r')},
		"z then escape":         {key('z'), special(tea.KeyEscape)},
		"z then tab":            {key('z'), special(tea.KeyTab)},
	}

	for name, keys := range tests {
		t.Run(name, func(t *testing.T) {
			got, pending := feed(application.ModeNormal, keys...)

			if len(got) != 0 {
				t.Errorf("%q produced %v, want nothing", name, got)
			}

			if pending != PendingNone {
				t.Errorf("%q left %v waiting, want nothing", name, pending)
			}
		})
	}
}

// After a sequence has been completed the next one starts over, so the same
// prefix twice over is two requests rather than one and a stray key.
func TestResolveRepeatsASequence(t *testing.T) {
	got, pending := feed(application.ModeNormal, key('g'), key('g'), key('g'), key('g'))

	if len(got) != 2 {
		t.Fatalf("gggg produced %v, want two requests", got)
	}

	for i, act := range got {
		if act != (application.ActionMoveFirst{}) {
			t.Errorf("request %d = %v, want %v", i, act, application.ActionMoveFirst{})
		}
	}

	if pending != PendingNone {
		t.Errorf("gggg left %v waiting, want nothing", pending)
	}
}

// TestResolveQuitsFromEveryMode covers the one binding that is not a mode's
// to withhold. A mode reached from normal and binding nothing of its own
// would otherwise trap the session with no way out but killing the terminal.
func TestResolveQuitsFromEveryMode(t *testing.T) {
	want := application.ActionQuit{}

	for _, mode := range allModes {
		t.Run(mode.String(), func(t *testing.T) {
			got, pending := Resolve(ctrl('c'), mode, PendingNone)

			if got != want {
				t.Errorf("Resolve(ctrl+c, %v) = %v, want %v", mode, got, want)
			}

			if pending != PendingNone {
				t.Errorf("Resolve(ctrl+c, %v) left %v waiting", mode, pending)
			}
		})
	}
}

// Half a sequence is no reason to be unable to leave.
func TestResolveQuitsDuringAPrefix(t *testing.T) {
	got, pending := feed(application.ModeNormal, key('g'), ctrl('c'))

	assertOneAction(t, got, application.ActionQuit{})

	if pending != PendingNone {
		t.Errorf("left %v waiting, want nothing", pending)
	}
}

// Every mode but normal is unreachable so far, and none of them borrows the
// bindings of the one they will be entered from: a key pressed while editing
// belongs to the editor, not to the document.
func TestResolveOutsideNormalMode(t *testing.T) {
	for _, mode := range allModes {
		if mode == application.ModeNormal {
			continue
		}

		t.Run(mode.String(), func(t *testing.T) {
			got, pending := Resolve(key('q'), mode, PendingNone)

			if got != nil {
				t.Errorf("Resolve(q, %v) = %v, want nil", mode, got)
			}

			if pending != PendingNone {
				t.Errorf("Resolve(q, %v) left %v waiting", mode, pending)
			}
		})
	}
}

// A sequence started in normal mode does not survive into another: the key
// that would complete it means something else there.
func TestResolveDropsAPrefixOnAChangeOfMode(t *testing.T) {
	got, pending := Resolve(key('g'), application.ModeEdit, PendingG)

	if got != nil {
		t.Errorf("Resolve(g, edit) = %v, want nil", got)
	}

	if pending != PendingNone {
		t.Errorf("Resolve(g, edit) left %v waiting, want nothing", pending)
	}
}
