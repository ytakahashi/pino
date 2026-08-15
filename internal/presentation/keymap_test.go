package presentation

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application"
)

// The keystrokes below are built the way a terminal reports them, since the
// table is matched against their textual form and that is what produces it.

func TestResolveMapsNormalModeKeysToActions(t *testing.T) {
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

		// One key for editing, whatever is selected: what acting on a node
		// means is decided where the document is.
		{name: "enter edits", key: special(tea.KeyEnter), want: application.ActionEdit{}},
		{name: "r renames a key", key: key('r'), want: application.ActionRenameKey{}},
		{name: "a adds a child", key: key('a'), want: application.ActionAddChild{}},
		{name: "A adds a sibling", key: shifted('a', 'A'), want: application.ActionAddSibling{}},
		{name: "d deletes", key: key('d'), want: application.ActionDelete{}},
		{name: "t changes a type", key: key('t'), want: application.ActionChangeType{}},
		{name: "u undoes", key: key('u'), want: application.ActionUndo{}},
		{name: "ctrl+r redoes", key: ctrl('r'), want: application.ActionRedo{}},

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
		"g then delete":         {key('g'), key('d')},
		"g then an unbound one": {key('g'), key('x')},
		"g then escape":         {key('g'), special(tea.KeyEscape)},
		"g then tab":            {key('g'), special(tea.KeyTab)},
		"z then a bound key":    {key('z'), key('l')},
		"z then delete":         {key('z'), key('d')},
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
func TestResolveIgnoresNormalModeKeysOutsideNormalMode(t *testing.T) {
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

// What a key means while a list of choices is on screen. The list comes from
// the prompt, because it is the prompt that wrote the keys down.
func TestResolveChoiceMapsKeysToAnswers(t *testing.T) {
	t.Parallel()

	p := application.PromptInfo{
		Kind:    application.PromptChoice,
		Choices: []application.Choice{{Key: 's', Label: "string"}, {Key: 'z', Label: "null"}},
	}

	tests := map[string]struct {
		key  tea.KeyPressMsg
		want application.Action
	}{
		"a key on offer":         {key('s'), application.ActionPromptChoose{Key: 's'}},
		"another key on offer":   {key('z'), application.ActionPromptChoose{Key: 'z'}},
		"escape withdraws":       {special(tea.KeyEscape), application.ActionCancel{}},
		"a key not on offer":     {key('n'), nil},
		"a key bound elsewhere":  {key('j'), nil},
		"the way out of a table": {key('q'), nil},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := ResolveChoice(tc.key, p); got != tc.want {
				t.Errorf("ResolveChoice(%q) = %v, want %v", tc.key.String(), got, tc.want)
			}
		})
	}
}

// A prompt offering nothing accepts nothing but the way out, which is what
// keeps a question from becoming a dead end however it was built.
func TestResolveChoiceOffersNothingOfItsOwn(t *testing.T) {
	t.Parallel()

	empty := application.PromptInfo{Kind: application.PromptChoice}

	if got := ResolveChoice(key('y'), empty); got != nil {
		t.Errorf("ResolveChoice(y) = %v, want nil", got)
	}

	if got := ResolveChoice(special(tea.KeyEscape), empty); got != (application.ActionCancel{}) {
		t.Errorf("ResolveChoice(esc) = %v, want a cancellation", got)
	}
}

// The inspector and the application derive what a node can be asked to do
// independently: one from the fields the pane already carries, the other from
// the tree itself. Walk every node of every shape a document takes and press
// all six keys, so that none can be advertised where nothing happens, or
// hidden where something does.
//
// Every key is asked about rather than only the ones whose availability
// varies. A key that is always offered is the one an exception can hide in,
// since nothing is left to compare it against.
func TestAvailableOperationsAgreeWithTheApplication(t *testing.T) {
	t.Parallel()

	operations := editingOperations()

	for name, root := range editingDocuments(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			probe := openApp(t, root)

			// Walking with the cursor rather than over the tree keeps the
			// nodes the ones a reader can reach: an ordinal is how many times
			// j was pressed, which is what appAt repeats on a session of its
			// own.
			for ordinal := 0; ; ordinal++ {
				info := probe.Inspector()
				keys := available(info)

				for _, op := range operations {
					got := slices.Contains(keys, op.spelling)
					want := changesTheSession(appAt(t, root, ordinal), op.act)

					if got != want {
						t.Errorf("node %q advertises %s = %t, pressing it changes the session = %t",
							info.Pointer, op.spelling, got, want)
					}
				}

				before := info.Pointer
				probe.Do(application.ActionMoveNext{})
				if probe.Inspector().Pointer == before {
					break
				}
			}
		})
	}
}

// The pane and the key table hold the spelling of a key separately. Press what
// the pane says it is offering and see the operation it was offering, so that
// a key rebound in one place and left alone in the other cannot go on reading
// as an offer.
func TestAdvertisedKeysResolveToTheOperationsTheyName(t *testing.T) {
	t.Parallel()

	for _, op := range editingOperations() {
		// The spelling is the one a terminal gives the press, which is what
		// lets the table above be searched for it by reading.
		if got := op.press.String(); got != op.spelling {
			t.Errorf("the key advertised as %q is sent as %q", op.spelling, got)
		}

		got, pending := Resolve(op.press, application.ModeNormal, PendingNone)
		if got != op.act {
			t.Errorf("Resolve(%q) = %v, want %v", op.spelling, got, op.act)
		}

		// An editing key is complete in itself. One that left a prefix waiting
		// would be offered on a row it takes two presses to accept.
		if pending != PendingNone {
			t.Errorf("Resolve(%q) leaves %v waiting, want nothing", op.spelling, pending)
		}
	}
}

// A key is written on screen the way it is named rather than the way a
// terminal spells it, and a key that is a character is written as it stands:
// a and A ask for different things, and the case is the whole difference.
func TestKeyLabelsNameTheKeysWithoutRespellingThem(t *testing.T) {
	t.Parallel()

	got := keyLabels([]string{"enter", "t", "a", "A", "ctrl+r"})
	want := []string{"Enter", "t", "a", "A", "Ctrl+r"}

	if !slices.Equal(got, want) {
		t.Errorf("keyLabels() = %v, want %v", got, want)
	}
}
