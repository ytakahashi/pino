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

func TestResolveInNormalMode(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
		want application.Action
	}{
		{
			name: "q quits",
			key:  tea.KeyPressMsg{Code: 'q', Text: "q"},
			want: application.ActionQuit{},
		},
		{
			// Shifted keys carry their own meanings in vim, so Q is not q
			// with a modifier that can be ignored.
			name: "shifted q is not bound",
			key:  tea.KeyPressMsg{Code: 'q', Mod: tea.ModShift, ShiftedCode: 'Q', Text: "Q"},
			want: nil,
		},
		{
			name: "unbound letter",
			key:  tea.KeyPressMsg{Code: 'j', Text: "j"},
			want: nil,
		},
		{
			name: "unbound special key",
			key:  tea.KeyPressMsg{Code: tea.KeyEscape},
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resolve(tc.key, application.ModeNormal); got != tc.want {
				t.Errorf("Resolve(%q) = %v, want %v", tc.key.String(), got, tc.want)
			}
		})
	}
}

// TestResolveQuitsFromEveryMode covers the one binding that is not a mode's
// to withhold. A mode reached from normal and binding nothing of its own
// would otherwise trap the session with no way out but killing the terminal.
func TestResolveQuitsFromEveryMode(t *testing.T) {
	want := application.ActionQuit{}

	for _, mode := range allModes {
		t.Run(mode.String(), func(t *testing.T) {
			key := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}

			if got := Resolve(key, mode); got != want {
				t.Errorf("Resolve(ctrl+c, %v) = %v, want %v", mode, got, want)
			}
		})
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
			key := tea.KeyPressMsg{Code: 'q', Text: "q"}

			if got := Resolve(key, mode); got != nil {
				t.Errorf("Resolve(q, %v) = %v, want nil", mode, got)
			}
		})
	}
}
