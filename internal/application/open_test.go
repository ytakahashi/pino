package application

import (
	"errors"
	"testing"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// A path holding nothing is where a document begins rather than a failure to
// report. Nothing is written until the reader saves, so opening one must ask
// the store for nothing else.
func TestOpeningAPathThatHoldsNothingStartsAnEmptyDocument(t *testing.T) {
	t.Parallel()

	app, files := creating(t)

	if got := app.doc.Root().Kind(); got != domain.KindObject {
		t.Errorf("the document is a %v, want an object", got)
	}

	if n := app.doc.Root().(*domain.Object).Len(); n != 0 {
		t.Errorf("the document holds %d members, want none", n)
	}

	// Nothing has been typed, so there is nothing to save. New and dirty are
	// independent: the file has still to be created, and the document is what
	// it was opened as.
	if app.doc.IsDirty() {
		t.Error("an untouched new document is dirty")
	}

	if !app.Status().New {
		t.Error("a document at a path holding nothing is not reported as new")
	}

	if app.meta != nil {
		t.Error("a document that came from no file carries a Meta")
	}

	if got, want := app.format, domain.DefaultFormat(); got != want {
		t.Errorf("the layout is %#v, want the default %#v", got, want)
	}

	if len(files.writes) != 0 {
		t.Error("opening a path that holds nothing wrote to it")
	}
}

// Only a path holding nothing starts a document. A file that is there and
// cannot be read is a failure, and replacing its contents with an empty
// object is how a file is silently thrown away.
func TestOpeningReportsWhatCannotBeRead(t *testing.T) {
	t.Parallel()

	broken := errors.New("broken symbolic link")
	unreadable := errors.New("permission denied")
	notJSON := errors.New("unexpected end of JSON input")

	tests := map[string]struct {
		files  *fakeFileStore
		parser *fakeParser
		want   error
	}{
		// The store tells a link pointing at nothing from a path that is
		// free, which is what keeps this out of the case above.
		"a link to nothing": {
			files:  &fakeFileStore{err: broken},
			parser: &fakeParser{},
			want:   broken,
		},
		"a file that cannot be read": {
			files:  &fakeFileStore{err: unreadable},
			parser: &fakeParser{},
			want:   unreadable,
		},
		// A file that exists and holds no JSON, which an empty file is. It is
		// the parser's to refuse, with a position.
		"a file holding nothing": {
			files:  &fakeFileStore{data: map[string][]byte{"config.json": {}}},
			parser: &fakeParser{err: notJSON},
			want:   notJSON,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := New(Deps{
				Parser:   tc.parser,
				Files:    tc.files,
				JSONView: &fakeRenderer{},
				TreeView: &fakeRenderer{},
			}, Config{})

			if err := app.Open("config.json"); !errors.Is(err, tc.want) {
				t.Fatalf("Open error = %v, want %v", err, tc.want)
			}

			if app.doc != nil {
				t.Error("a document is open although the path could not be read")
			}

			if app.Status().New {
				t.Error("a path that could not be read is reported as a new document")
			}
		})
	}
}

// The width the command line asked for wins wherever a layout comes from:
// a file that uses another one, a file that does not exist, and a file read
// again after it changed.
func TestAnIndentGivenOnTheCommandLineWins(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"four spaces":      "    ",
		"a tab":            "\t",
		"no indent at all": "",
	}

	for name, indent := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cfg := Config{IndentOverride: indent, OverrideIndent: true}

			// The file uses four spaces, and a new document would default to
			// two, so neither can be mistaken for the override.
			files := &fakeFileStore{
				data:    map[string][]byte{savePath: []byte(testSource)},
				meta:    openMeta,
				outcome: WriteOutcome{Meta: writtenMeta, Committed: true},
			}

			parser := &fakeParser{root: sample(t)}

			app := New(Deps{
				Parser:   parser,
				Files:    files,
				JSONView: documentview.NewJSONRenderer(),
				TreeView: documentview.NewTreeRenderer(),
			}, cfg)

			if err := app.Open(savePath); err != nil {
				t.Fatalf("Open: %v", err)
			}

			if got := app.Status().Indent; got != indent {
				t.Errorf("the layout of a file that was read is %q, want %q", got, indent)
			}

			// A document with no file to take a layout from.
			fresh := New(Deps{
				Parser:   &fakeParser{},
				Files:    &fakeFileStore{data: map[string][]byte{}},
				JSONView: documentview.NewJSONRenderer(),
				TreeView: documentview.NewTreeRenderer(),
			}, cfg)

			if err := fresh.Open("new.json"); err != nil {
				t.Fatalf("Open: %v", err)
			}

			if got := fresh.Status().Indent; got != indent {
				t.Errorf("the layout of a new document is %q, want %q", got, indent)
			}

			// And again after the file is read a second time.
			files.status = ChangeModified

			editValue(t, app, "/server/host", "127.0.0.1")
			parser.parse = func([]byte, domain.Dialect) (domain.Node, error) { return app.doc.Root(), nil }

			press(app, ActionSave{})
			reloadTo(t, app, testdocsOutside(t))
			pick(app, 'r')

			if got := app.Status().Indent; got != indent {
				t.Errorf("the layout after reloading is %q, want %q", got, indent)
			}
		})
	}
}

// Without the flag the file decides, which is what keeps a save from
// reformatting lines nobody touched.
func TestWithoutAnIndentTheFileDecides(t *testing.T) {
	t.Parallel()

	app, _ := saving(t, sample(t))

	if got, want := app.Status().Indent, "    "; got != want {
		t.Errorf("the layout is %q, want the %q the file uses", got, want)
	}
}

// Opening a second document must leave nothing of the first, including what
// the store recorded about it.
func TestOpeningAgainForgetsTheFileBefore(t *testing.T) {
	t.Parallel()

	app, _ := saving(t, sample(t))

	if err := app.Open("new.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	if !app.Status().New {
		t.Error("a path holding nothing is not reported as new after another file was open")
	}

	if app.meta != nil {
		t.Error("the session kept the Meta of the file it had open before")
	}
}
