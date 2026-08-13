package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// What the session reports about the selection for the bar to draw.

func TestStatusFollowsTheCursor(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	// Every kind a value can be, reached by walking the document.
	tests := []struct {
		steps   []Action
		pointer string
		typ     string
	}{
		{steps: nil, pointer: "", typ: "object"},
		{steps: []Action{ActionMoveNext{}}, pointer: "/name", typ: "string"},
		{steps: []Action{ActionMoveNext{}}, pointer: "/server", typ: "object"},
		{steps: []Action{ActionMoveIn{}, ActionMoveNext{}}, pointer: "/server/ports", typ: "array"},
		{steps: []Action{ActionMoveIn{}}, pointer: "/server/ports/0", typ: "number"},
		{steps: []Action{ActionMoveNext{}, ActionMoveNext{}}, pointer: "/server/tls", typ: "boolean"},
	}

	for _, tt := range tests {
		press(app, tt.steps...)

		info := app.Status()

		if info.Pointer != tt.pointer {
			t.Fatalf("Status().Pointer = %q, want %q", info.Pointer, tt.pointer)
		}

		if info.Type != tt.typ {
			t.Errorf("Status().Type = %q at %q, want %q", info.Type, tt.pointer, tt.typ)
		}
	}
}

func TestStatusDescribesANullValue(t *testing.T) {
	t.Parallel()

	app := session(t, object(t, member("nothing", domain.NewNull())))

	app.Do(ActionMoveNext{})

	if got := app.Status().Type; got != "null" {
		t.Errorf("Status().Type = %q, want null", got)
	}
}

// Nothing is selected before a document is open, and an empty pointer alone
// cannot say so: the root has one too.
func TestStatusIsEmptyWithoutADocument(t *testing.T) {
	t.Parallel()

	info := New(Deps{JSONView: NewJSONRenderer(), TreeView: NewTreeRenderer()}).Status()

	if info.Pointer != "" {
		t.Errorf("Status().Pointer = %q with no document open, want empty", info.Pointer)
	}

	if info.Type != "" {
		t.Errorf("Status().Type = %q with no document open, want empty", info.Type)
	}
}

// Asking for the status must not lay the document out: the bar is refreshed on
// every frame, and drawing it a second time is what a row count would have
// cost.
func TestStatusDoesNotRender(t *testing.T) {
	t.Parallel()

	renderer := &fakeRenderer{lines: []Line{{Kind: LineOpen}}}
	app := New(Deps{
		Parser:   &fakeParser{root: sample(t)},
		Files:    fakeFileStore{data: map[string][]byte{"a.json": []byte(testSource)}},
		JSONView: renderer,
		TreeView: renderer,
	})

	if err := app.Open("a.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	before := renderer.calls
	_ = app.Status()

	if renderer.calls != before {
		t.Errorf("Status() rendered %d times, want none", renderer.calls-before)
	}
}
