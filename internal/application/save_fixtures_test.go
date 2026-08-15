package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// The helpers here open a session that can be saved: a store with a file in
// it, and a parser that reads back what was written.

// savePath is the file the sessions below are opened from.
const savePath = "config.json"

// openMeta and writtenMeta stand for what a store records about a file before
// and after a save. They are pointers so that a test can say which of the two
// the session is carrying.
var (
	openMeta    = &fakeMeta{}
	writtenMeta = &fakeMeta{}
)

// saving opens a document from a store that answers as a save needs.
//
// The parser reads whatever is encoded back as the very tree it came from.
// That the encoder and the parser really do agree is not this layer's to
// prove — domain checks the encoding, and the parser adapter checks the round
// trip — and a fake that tried to would be a second implementation of both.
// What is left here is what the session does with each answer.
func saving(t *testing.T, root domain.Node) (*App, *fakeFileStore) {
	t.Helper()

	files := &fakeFileStore{
		data:    map[string][]byte{savePath: []byte(testSource)},
		meta:    openMeta,
		outcome: WriteOutcome{Meta: writtenMeta, Committed: true},
	}

	app := openWith(t, files, root, savePath)

	return app, files
}

// creating opens a document at a path the store holds nothing at.
func creating(t *testing.T) (*App, *fakeFileStore) {
	t.Helper()

	files := &fakeFileStore{
		data:    map[string][]byte{},
		outcome: WriteOutcome{Meta: writtenMeta, Committed: true},
	}

	app := openWith(t, files, nil, "new.json")

	return app, files
}

// openWith opens path through files, with the real renderers.
//
// root is what the parser hands back for the file; a new document has none,
// and the empty object it starts from is built by the session itself.
func openWith(t *testing.T, files *fakeFileStore, root domain.Node, path string) *App {
	t.Helper()

	parser := &fakeParser{root: root}

	app := New(Deps{
		Parser:   parser,
		Files:    files,
		JSONView: documentview.NewJSONRenderer(),
		TreeView: documentview.NewTreeRenderer(),
	}, Config{})

	if err := app.Open(path); err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}

	// Set after opening: what a save encodes is the tree as it stands then,
	// which is not the one the file was read as once anything is edited.
	parser.parse = func([]byte, domain.Dialect) (domain.Node, error) { return app.doc.Root(), nil }

	return app
}

// parserOf is the parser a session was built with.
func parserOf(t *testing.T, a *App) *fakeParser {
	t.Helper()

	p, ok := a.deps.Parser.(*fakeParser)
	if !ok {
		t.Fatalf("the session was built with a %T", a.deps.Parser)
	}

	return p
}

// describeAction names an action for a subtest.
func describeAction(act Action) string {
	if _, ok := act.(ActionCancel); ok {
		return "with Esc"
	}

	return "with the key the prompt offers"
}

// testdocsOutside is a document standing in for what another program left in
// the file: the same shape, different values, so that a reload can be told
// from the session having kept what it had.
func testdocsOutside(t *testing.T) domain.Node {
	t.Helper()

	root, err := domain.NewObject([]domain.Member{
		{Key: "name", Value: text(t, "pino")},
		{Key: "elsewhere", Value: domain.NewBool(true)},
	})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	return root
}

// promptKeys is the keys a prompt says it accepts.
func promptKeys(info PromptInfo) []rune {
	keys := make([]rune, 0, len(info.Choices))
	for _, c := range info.Choices {
		keys = append(keys, c.Key)
	}

	return keys
}

// editValue types text into the value the cursor is on and commits it, which
// is the shortest way to make a document dirty.
func editValue(t *testing.T, a *App, ptr, text string) {
	t.Helper()

	standOn(t, a, ptr)
	press(a, ActionEdit{})
	answer(a, text)

	if !a.doc.IsDirty() {
		t.Fatalf("editing %s left the document clean", ptr)
	}
}
