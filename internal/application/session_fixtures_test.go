package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// The helpers here open a session and drive it. Every test of a session
// starts from one, whether what it is about is moving, scrolling, folding or
// the views, so they live apart from all four.

// session opens root in the JSON view through the real renderers.
func session(t *testing.T, root domain.Node) *App {
	t.Helper()

	return sessionIn(t, root, ViewJSON)
}

// sessionIn opens root in a chosen view.
//
// The view is set after opening because opening builds a fresh view state, and
// it is set directly rather than by pressing Tab so that a test of movement
// does not depend on the action that switches views.
func sessionIn(t *testing.T, root domain.Node, view ViewMode) *App {
	t.Helper()

	app := New(Deps{
		Parser:   &fakeParser{root: root},
		Files:    &fakeFileStore{data: map[string][]byte{"a.json": []byte(testSource)}},
		JSONView: documentview.NewJSONRenderer(),
		TreeView: documentview.NewTreeRenderer(),
	}, Config{})

	if err := app.Open("a.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	app.view.ViewMode = view

	return app
}

// cursorOf is where the session is pointing, as a pointer.
func cursorOf(a *App) string { return a.view.Cursor.String() }

// press applies actions in order, the way a sequence of keystrokes would.
func press(a *App, actions ...Action) {
	for _, act := range actions {
		a.Do(act)
	}
}

// repeat is one action pressed over and over.
func repeat(act Action, n int) []Action {
	actions := make([]Action, n)
	for i := range actions {
		actions[i] = act
	}

	return actions
}

// pointersOf is what a session draws, as pointers, in order.
func pointersOf(a *App) []string {
	frame := a.Frame()

	got := make([]string, 0, len(frame.Lines))
	for _, l := range frame.Lines {
		if l.Kind != documentview.LineClose {
			got = append(got, l.Path.String())
		}
	}

	return got
}
