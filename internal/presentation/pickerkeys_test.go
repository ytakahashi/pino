package presentation

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
	"github.com/ytakahashi/pino/internal/application/picker"
)

func TestPickerKeysResolveToTheRequestsTheyAdvertise(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		key    tea.KeyPressMsg
		action picker.Action
	}{
		{key('j'), picker.ActionMoveNext{}},
		{special(tea.KeyDown), picker.ActionMoveNext{}},
		{key('k'), picker.ActionMovePrev{}},
		{special(tea.KeyUp), picker.ActionMovePrev{}},
		{shifted('g', 'G'), picker.ActionMoveLast{}},
		{ctrl('d'), picker.ActionScrollHalfDown{}},
		{ctrl('u'), picker.ActionScrollHalfUp{}},
		{key('/'), picker.ActionBeginFilter{}},
		{special(tea.KeyEnter), picker.ActionOpen{}},
		{special(tea.KeyEscape), picker.ActionClearFilter{}},
		{key('?'), picker.ActionShowHelp{}},
		{key('q'), picker.ActionQuit{}},
	} {
		t.Run(tc.key.String(), func(t *testing.T) {
			t.Parallel()
			act, terminal, pending := resolvePicker(tc.key, picker.ModeNormal, PendingNone)
			if act != tc.action || terminal != TerminalNone || pending != PendingNone {
				t.Errorf("resolved to (%v, %v, %v), want (%v, none, none)", act, terminal, pending, tc.action)
			}
		})
	}
	act, terminal, pending := resolvePicker(key('m'), picker.ModeNormal, PendingNone)
	if act != nil || terminal != TerminalToggleMouse || pending != PendingNone {
		t.Error("m did not request mouse toggling")
	}
	for _, k := range []tea.KeyPressMsg{key('n'), shifted('n', 'N'), key('h'), key('l')} {
		act, terminal, pending := resolvePicker(k, picker.ModeNormal, PendingNone)
		if act != nil || terminal != TerminalNone || pending != PendingNone {
			t.Errorf("%q has an unexpected binding", k.String())
		}
	}
}

// Moving through a list of files is moving through a document's rows, so a
// reader who has learned one has learned the other.
func TestPickerMovesWithTheEditorKeys(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		picker picker.Action
		editor application.Action
	}{
		{picker.ActionMoveNext{}, application.ActionMoveNext{}},
		{picker.ActionMovePrev{}, application.ActionMovePrev{}},
		{picker.ActionMoveLast{}, application.ActionMoveLast{}},
		{picker.ActionScrollHalfDown{}, application.ActionScrollHalfDown{}},
		{picker.ActionScrollHalfUp{}, application.ActionScrollHalfUp{}},
	} {
		got := pickerBindingFor(t, tc.picker)
		want := normalBindingFor(t, tc.editor)
		if !slices.Equal(got.Keys, want.Keys) || got.HelpKeys != want.HelpKeys {
			t.Errorf("%T is bound to %q (%s), want the editor's %q (%s)", tc.picker, got.Keys, got.HelpKeys, want.Keys, want.HelpKeys)
		}
	}

	for _, b := range pendingBindings {
		if b.Action != (application.ActionMoveFirst{}) {
			continue
		}

		got := pickerFirstBinding
		if b.Prefix != PendingG || !slices.Equal(got.Keys, b.Keys) || got.HelpKeys != b.HelpKeys {
			t.Errorf("the first file is %s, want the editor's %s", got.HelpKeys, b.HelpKeys)
		}

		return
	}

	t.Fatal("the editor has no binding for the first row")
}

func TestPickerPrefixesCompleteOrCancelWithoutRetryingTheKey(t *testing.T) {
	t.Parallel()

	act, _, pending := resolvePicker(key('g'), picker.ModeNormal, PendingNone)
	if act != nil || pending != PendingG {
		t.Fatal("g did not wait for its second key")
	}
	act, _, pending = resolvePicker(key('g'), picker.ModeNormal, pending)
	if act != (picker.ActionMoveFirst{}) || pending != PendingNone {
		t.Fatal("gg did not request the first file")
	}
	for _, k := range []tea.KeyPressMsg{key('j'), key('m'), special(tea.KeyEscape)} {
		act, terminal, pending := resolvePicker(k, picker.ModeNormal, PendingG)
		if act != nil || terminal != TerminalNone || pending != PendingNone {
			t.Errorf("g%s did not cancel quietly", k.String())
		}
	}
}

func TestPickerCtrlCQuitsInEveryModeAndDuringAPrefix(t *testing.T) {
	t.Parallel()

	for _, mode := range []picker.Mode{picker.ModeNormal, picker.ModeFilter, picker.ModeHelp} {
		m, _ := picking(t, "a.json")
		switch mode {
		case picker.ModeFilter:
			m = updateRoot(t, m, key('/'))
		case picker.ModeHelp:
			m = updateRoot(t, m, key('?'))
		case picker.ModeNormal:
			m = updateRoot(t, m, key('g'))
		}
		_, cmd := m.Update(ctrl('c'))
		if cmd == nil || !isQuit(cmd()) {
			t.Errorf("Ctrl+C did not quit in mode %v", mode)
		}
	}
}

func TestPickerHelpNamesEveryBindingOnceAndFitsTheSmallestTerminal(t *testing.T) {
	t.Parallel()

	lines := pickerHelpLines()
	bindings := append([]pickerBinding{pickerFirstBinding}, pickerBindings...)
	for _, b := range bindings {
		count := 0
		for _, line := range lines {
			for _, e := range line.Entries {
				if e.Keys == b.HelpKeys && e.Description == b.Description {
					count++
				}
			}
		}
		if b.HelpKeys == "" || b.Description == "" || count != 1 {
			t.Errorf("%q %q appears %d times, want one documented binding", b.HelpKeys, b.Description, count)
		}
	}
	if len(lines)+1 > minHeight-statusBarRows {
		t.Fatal("help is taller than the smallest terminal")
	}
	for _, line := range lines {
		written := pad(line.Heading, helpLabelWidth)
		for i, e := range line.Entries {
			if i > 0 {
				written += helpEntryGap
			}
			written += e.Keys + " " + e.Description
		}
		if ansi.StringWidth(written) > minWidth {
			t.Errorf("help row is too wide: %q", written)
		}
	}
	m, _ := picking(t, "a.json")
	m = updateRoot(t, m, key('?'))
	screen := strings.Join(rootRows(m), "\n")
	for _, entry := range []string{"wheel scroll", "m select", "q/Ctrl+c quit", "Enter accept", "Esc cancel"} {
		if strings.Count(screen, entry) != 1 {
			t.Errorf("help does not show %q once", entry)
		}
	}
	for _, label := range keyLabels(helpClose) {
		if !strings.Contains(rootRows(m)[0], label) {
			t.Errorf("help title does not offer %q", label)
		}
	}
	assertRootSize(t, m, minWidth, minHeight)
}
