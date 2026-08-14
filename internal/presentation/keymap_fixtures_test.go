package presentation

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application"
	"github.com/ytakahashi/pino/internal/domain"
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

// editingDocument contains every relationship that changes which structural
// editing operations apply: the root, object members, array elements, scalars,
// and containers.
func editingDocument(t *testing.T) domain.Node {
	t.Helper()

	inner, err := domain.NewObject([]domain.Member{{Key: "value", Value: domain.NewNumber("1")}})
	if err != nil {
		t.Fatalf("NewObject(inner) = %v", err)
	}

	empty, err := domain.NewObject(nil)
	if err != nil {
		t.Fatalf("NewObject(empty) = %v", err)
	}

	text, err := domain.NewString("item")
	if err != nil {
		t.Fatalf("NewString() = %v", err)
	}

	root, err := domain.NewObject([]domain.Member{
		{Key: "object", Value: inner},
		{Key: "array", Value: domain.NewArray([]domain.Node{text, empty})},
		{Key: "number", Value: domain.NewNumber("2")},
	})
	if err != nil {
		t.Fatalf("NewObject(root) = %v", err)
	}

	return root
}

func appAt(t *testing.T, root domain.Node, ordinal int) *application.App {
	t.Helper()

	a := openApp(t, root)
	for range ordinal {
		a.Do(application.ActionMoveNext{})
	}

	return a
}

// acceptsConditionalOperation reports whether the application starts or
// commits one of the operations whose availability depends on the selected
// node. Each call receives a fresh session, so an immediate deletion and a
// prompt are equally observable without one operation affecting the next.
func acceptsConditionalOperation(a *application.App, act application.Action) bool {
	a.Do(act)

	return a.Mode() != application.ModeNormal || a.Status().Dirty
}
