package application

import (
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// The helpers here drive an edit from outside the session: standing on a node,
// answering whatever prompt is open, and reading back what the session did
// with the answer.

// standOn puts the cursor on pointer, where moving there with the keys would
// have left it.
//
// The path is parsed from text rather than built segment by segment, so that a
// test names the node the way the status bar does.
func standOn(t *testing.T, a *App, pointer string) {
	t.Helper()

	p, err := domain.ParsePointer(pointer)
	if err != nil {
		t.Fatalf("ParsePointer(%q): %v", pointer, err)
	}

	if _, ok := domain.Resolve(a.doc.Root(), p); !ok {
		t.Fatalf("%q is not in the document", pointer)
	}

	a.view.Cursor = p
	a.settle(a.render())
}

// pointer is a JSON Pointer as a path.
func pointer(t *testing.T, s string) domain.Path {
	t.Helper()

	p, err := domain.ParsePointer(s)
	if err != nil {
		t.Fatalf("ParsePointer(%q): %v", s, err)
	}

	return p
}

// nodeAt is what the document holds at pointer, and fails when it holds
// nothing there.
func nodeAt(t *testing.T, a *App, s string) domain.Node {
	t.Helper()

	n, ok := domain.Resolve(a.doc.Root(), pointer(t, s))
	if !ok {
		t.Fatalf("%q is not in the document", s)
	}

	return n
}

// answer types text into an open prompt and presses Enter, which is the pair
// of actions a widget sends: what is being typed, and then the whole of it.
func answer(a *App, text string) {
	a.Do(ActionPromptChange{Text: text})
	a.Do(ActionPromptSubmit{Text: text})
}

// pick presses one of the keys a choice prompt offers.
func pick(a *App, key rune) { a.Do(ActionPromptChoose{Key: key}) }

// beginInput is the text box an action asked for, and fails when it asked for
// none.
func beginInput(t *testing.T, effects []Effect) EffectBeginInput {
	t.Helper()

	for _, e := range effects {
		if in, ok := e.(EffectBeginInput); ok {
			return in
		}
	}

	t.Fatal("no text box was asked for")

	return EffectBeginInput{}
}

// versions is how many versions of the document the session is holding.
func versions(a *App) int { return len(a.history.entries) }

// contentOf is the document as one renderer draws it in full.
//
// Two sessions hold trees built separately, so the same edit made in each
// leaves them with equal documents and different objects: what can be compared
// between them is what the trees say. The folded set is left out on purpose —
// the document is what is being compared, and how each session is looking at
// it is compared beside this.
func contentOf(a *App) string {
	return dumpLines(NewJSONRenderer().Render(a.doc.Root(), RenderOptions{}))
}

// keysOf is the keys a choice prompt offers, in the order it offers them.
func keysOf(p PromptInfo) string {
	var b strings.Builder

	for _, c := range p.Choices {
		b.WriteRune(c.Key)
	}

	return b.String()
}

// longString is a document holding a string too long to be drawn in full,
// together with that string.
func longString(t *testing.T) (domain.Node, string) {
	t.Helper()

	value := strings.Repeat("pino ", 40)

	return object(t, member("motto", text(t, value))), value
}

// nested is a document with one member holding one member, which is the
// smallest subtree a confirmation can count.
func nested(t *testing.T) domain.Node {
	t.Helper()

	return object(t,
		member("a", object(t, member("b", text(t, "c")))),
		member("empty", object(t)),
	)
}
