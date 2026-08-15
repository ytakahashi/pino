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

// editingOperation is one thing the inspector can offer to do, written out as
// the key it is advertised as, the press that asks for it, and the request
// that press becomes.
type editingOperation struct {
	// spelling is the key the table gives the operation, which is what the
	// pane advertises it as.
	spelling string

	// press is what a terminal sends when that key is struck.
	press tea.KeyPressMsg

	// act is what the application is asked to do about it.
	act application.Action
}

// editingOperations is every operation the inspector can offer.
//
// The knowledge is held in three places — available says which operations a
// node is offered, the key table says which key asks for one and what a press
// of it means, and Do says what happens then — so the tests check each of them
// against this one list rather than against one another. A row that agreed
// with itself in two places and not the third would otherwise read as
// agreement.
func editingOperations() []editingOperation {
	return []editingOperation{
		{spelling: "enter", press: special(tea.KeyEnter), act: application.ActionEdit{}},
		{spelling: "t", press: key('t'), act: application.ActionChangeType{}},
		{spelling: "a", press: key('a'), act: application.ActionAddChild{}},
		{spelling: "A", press: shifted('a', 'A'), act: application.ActionAddSibling{}},
		{spelling: "d", press: key('d'), act: application.ActionDelete{}},
		{spelling: "r", press: key('r'), act: application.ActionRenameKey{}},
	}
}

// editingDocuments is what to walk to see every answer the editing keys have.
//
// The first holds every relationship that changes which operations apply: the
// root, object members, array elements, scalars, and containers both populated
// and empty. The other two are documents whose root is not a populated object,
// which is the shape the first one cannot take: they are what says that a
// container is asked about before the way it is named.
func editingDocuments(t *testing.T) map[string]domain.Node {
	t.Helper()

	return map[string]domain.Node{
		"a document of every relationship": everyRelationshipDocument(t),

		// A document is a JSON value, not necessarily an object. Enter types
		// over this one as it would over any number, so a rule reading only
		// that the root is named by nothing would hide a key that works.
		"a document holding one scalar": domain.NewNumber("1"),

		// The shape a new file starts in: a container that is the root and is
		// empty at once, which is both of the ways a fold can be missing.
		"a document holding nothing": newObject(t, nil),
	}
}

func everyRelationshipDocument(t *testing.T) domain.Node {
	t.Helper()

	text, err := domain.NewString("item")
	if err != nil {
		t.Fatalf("NewString() = %v", err)
	}

	inner := newObject(t, []domain.Member{{Key: "value", Value: domain.NewNumber("1")}})
	empty := newObject(t, nil)

	return newObject(t, []domain.Member{
		{Key: "object", Value: inner},
		{Key: "array", Value: domain.NewArray([]domain.Node{text, empty})},
		{Key: "number", Value: domain.NewNumber("2")},
	})
}

func newObject(t *testing.T, members []domain.Member) domain.Node {
	t.Helper()

	obj, err := domain.NewObject(members)
	if err != nil {
		t.Fatalf("NewObject() = %v", err)
	}

	return obj
}

func appAt(t *testing.T, root domain.Node, ordinal int) *application.App {
	t.Helper()

	a := openApp(t, root)
	for range ordinal {
		a.Do(application.ActionMoveNext{})
	}

	return a
}

// observation is everything a key press can change about a session, seen from
// outside it.
//
// The rows are counted because folding is the one answer an editing key gives
// that leaves the document, the mode and the cursor alone. Frame reports the
// whole document rather than the part that fits, so the count follows a fold
// without a terminal having said how tall it is.
type observation struct {
	mode    application.Mode
	dirty   bool
	rows    int
	pointer string
}

func observe(a *application.App) observation {
	status := a.Status()

	return observation{
		mode:    status.Mode,
		dirty:   status.Dirty,
		rows:    len(a.Frame().Lines),
		pointer: status.Pointer,
	}
}

// changesTheSession reports whether pressing a key does anything at all.
//
// Availability cannot be stated as "the application accepts it", because an
// operation refused and an operation carried out to no effect are the same
// thing from here: both return with nothing touched. What can be asked is
// whether the session moved, and that is what a row offering a key promises.
//
// Each call receives a fresh session, so a prompt, an immediate deletion and a
// fold are equally observable without one operation affecting the next.
func changesTheSession(a *application.App, act application.Action) bool {
	before := observe(a)

	a.Do(act)

	return observe(a) != before
}
