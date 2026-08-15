package application

import (
	"errors"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// Saving is the one thing pino does that cannot be undone from inside pino,
// so what these tests are about is which answers from the file store lead to
// the document being called saved.

func TestSaveWritesTheDocumentAndMarksItSaved(t *testing.T) {
	t.Parallel()

	app, files := saving(t, sample(t))
	editValue(t, app, "/server/ports/0", "8081")

	press(app, ActionSave{})

	if len(files.writes) != 1 {
		t.Fatalf("the store was asked to write %d times, want once", len(files.writes))
	}

	if files.writes[0] != savePath {
		t.Errorf("the document was written to %q, want %q", files.writes[0], savePath)
	}

	// What is written is the document as it stands, laid out the way the file
	// it came from was.
	if want := string(domain.Encode(app.doc.Root(), app.format)); string(files.written[0]) != want {
		t.Errorf("the store was given %q, want %q", files.written[0], want)
	}

	if app.doc.IsDirty() {
		t.Error("the document is still dirty after being written")
	}

	if app.meta != Meta(writtenMeta) {
		t.Error("the session did not take the Meta the write handed back")
	}

	if app.Prompt().Kind != PromptNone {
		t.Errorf("a prompt is open after a save that worked: %q", app.Prompt().Title)
	}
}

// The check comes first, and the file is only written when nothing has
// happened to it.
func TestSaveAsksWhatBecameOfTheFileBeforeWriting(t *testing.T) {
	t.Parallel()

	app, files := saving(t, sample(t))
	editValue(t, app, "/server/ports/0", "8081")

	press(app, ActionSave{})

	if len(files.checks) != 1 || files.checks[0] != savePath {
		t.Errorf("the store was asked about %v, want one check of %q", files.checks, savePath)
	}
}

// A document that holds what its file holds is not written again. Laying it
// out afresh would rewrite lines the reader never touched.
func TestSaveDoesNothingForAnUnchangedDocument(t *testing.T) {
	t.Parallel()

	app, files := saving(t, sample(t))

	press(app, ActionSave{})

	if len(files.writes) != 0 || len(files.checks) != 0 {
		t.Errorf("the store was asked to check %v and write %v, want neither", files.checks, files.writes)
	}
}

// A document whose file is not there yet is written whether or not anything
// was typed into it: the file it would create does not exist.
func TestSaveCreatesTheFileOfANewDocument(t *testing.T) {
	t.Parallel()

	tests := map[string]func(t *testing.T, a *App){
		"untouched": func(*testing.T, *App) {},
		"edited": func(t *testing.T, a *App) {
			t.Helper()

			press(a, ActionAddChild{})
			answer(a, "host")
			pick(a, 's')
			answer(a, "localhost")
		},
	}

	for name, edit := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app, files := creating(t)

			if !app.Status().New {
				t.Fatal("a document opened at a path holding nothing is not reported as new")
			}

			edit(t, app)
			press(app, ActionSave{})

			if len(files.writes) != 1 {
				t.Fatalf("the store was asked to write %d times, want once", len(files.writes))
			}

			if app.Status().New {
				t.Error("the document is still reported as new after its file was written")
			}

			if app.doc.IsDirty() {
				t.Error("the document is dirty after being written")
			}
		})
	}
}

// A new document is checked against the file system too. The path being free
// when it was opened is a claim like any other, and a file created there
// since is somebody else's.
func TestSaveChecksThePathOfANewDocument(t *testing.T) {
	t.Parallel()

	app, files := creating(t)
	files.status = ChangeModified

	press(app, ActionSave{})

	if len(files.checks) != 1 {
		t.Fatalf("the store was asked about the path %d times, want once", len(files.checks))
	}

	if len(files.writes) != 0 {
		t.Error("a file that appeared at the path was overwritten")
	}

	if app.Mode() != ModeConfirm {
		t.Errorf("mode = %v, want the conflict to be put to the reader", app.Mode())
	}
}

// The file changed underneath the session. Nothing is written, and what to do
// is the reader's to say.
func TestSaveStopsWhenTheFileChangedOutside(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		status ChangeStatus
		title  string
	}{
		"modified": {status: ChangeModified, title: "The file has changed outside pino."},
		"deleted":  {status: ChangeDeleted, title: "The file was deleted outside pino."},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app, files := saving(t, sample(t))
			files.status = tc.status

			editValue(t, app, "/server/ports/0", "8081")
			press(app, ActionSave{})

			if len(files.writes) != 0 {
				t.Error("the document was written over a file that had changed")
			}

			if !app.doc.IsDirty() {
				t.Error("the document was marked saved although nothing was written")
			}

			info := app.Prompt()
			if info.Title != tc.title {
				t.Errorf("the prompt reads %q, want %q", info.Title, tc.title)
			}

			if got, want := promptKeys(info), []rune{'r', 'o', 'c'}; string(got) != string(want) {
				t.Errorf("the prompt offers %q, want %q", string(got), string(want))
			}
		})
	}
}

// Encoding is checked before the file system is touched at all: a defect
// there is pino's own, and finding it should cost nothing on disk.
func TestSaveWritesNothingWhenTheDocumentWouldNotSurviveEncoding(t *testing.T) {
	t.Parallel()

	other, err := domain.NewObject(nil)
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	parseErr := errors.New("the bytes are not JSON")

	tests := map[string]func(p *fakeParser){
		"the bytes cannot be parsed back": func(p *fakeParser) {
			p.parse = func([]byte, domain.Dialect) (domain.Node, error) { return nil, parseErr }
		},
		"the bytes are another document": func(p *fakeParser) {
			p.parse = func([]byte, domain.Dialect) (domain.Node, error) { return other, nil }
		},
	}

	for name, arrange := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app, files := saving(t, sample(t))
			editValue(t, app, "/server/ports/0", "8081")

			arrange(parserOf(t, app))

			press(app, ActionSave{})

			if len(files.checks) != 0 || len(files.writes) != 0 {
				t.Errorf("the store was asked to check %v and write %v, want neither", files.checks, files.writes)
			}

			if !app.doc.IsDirty() {
				t.Error("the document was marked saved although nothing was written")
			}

			if app.Prompt().Kind != PromptChoice || app.Status().Error == "" {
				t.Error("the failure was not put on screen")
			}
		})
	}
}

// A write that failed before the rename leaves everything as it was: the
// document is still unsaved, and the Meta still describes the file that is
// still there.
func TestSaveKeepsTheDocumentWhenTheWriteFailedBeforeCommitting(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("permission denied")

	app, files := saving(t, sample(t))
	files.outcome, files.writeErr = WriteOutcome{}, writeErr

	editValue(t, app, "/server/ports/0", "8081")
	press(app, ActionSave{})

	if !app.doc.IsDirty() {
		t.Error("the document was marked saved although the file was not replaced")
	}

	if app.meta != Meta(openMeta) {
		t.Error("the session dropped the Meta of the file that is still there")
	}

	if got := app.Status().Error; got != writeErr.Error() {
		t.Errorf("the bar reads %q, want the reason the write failed", got)
	}
}

// The rename happened and something after it did not. What is at the path is
// the new text, so the document is saved — and the failure is still reported,
// because nobody told the directory.
func TestSaveMarksADocumentSavedWhenTheRenameCommitted(t *testing.T) {
	t.Parallel()

	syncErr := errors.New("the directory could not be synced")

	app, files := saving(t, sample(t))
	files.writeErr = syncErr

	editValue(t, app, "/server/ports/0", "8081")
	press(app, ActionSave{})

	if app.doc.IsDirty() {
		t.Error("the document is dirty although the file was replaced")
	}

	if app.meta != Meta(writtenMeta) {
		t.Error("the session did not take the Meta of the file it wrote")
	}

	if got := app.Status().Error; got != syncErr.Error() {
		t.Errorf("the bar reads %q, want the reason the write could not be confirmed", got)
	}
}

// A check answering with a state the port does not define is a defect in the
// store, not a change to the file. Putting it to the reader as one would
// offer an Overwrite that skips the check whose answer could not be read.
func TestSaveRefusesAStateThePortDoesNotDefine(t *testing.T) {
	t.Parallel()

	app, files := saving(t, sample(t))
	files.status = ChangeDeleted + 1

	editValue(t, app, "/server/ports/0", "8081")
	press(app, ActionSave{})

	if len(files.writes) != 0 {
		t.Error("the document was written although what became of the file was not known")
	}

	if !app.doc.IsDirty() {
		t.Error("the document was marked saved although nothing was written")
	}

	// The error prompt rather than the conflict one: there is no Overwrite to
	// offer for a state nobody could read.
	if got, want := promptKeys(app.Prompt()), []rune{'o'}; string(got) != string(want) {
		t.Errorf("the prompt offers %q, want %q", string(got), string(want))
	}

	if got := app.Status().Error; got != errStoreStatus.Error() {
		t.Errorf("the bar reads %q, want the store's answer refused", got)
	}
}

// The port allows three answers. Anything else is a defect in the store, and
// believing half of it is how a document comes to be called saved when it is
// not.
func TestSaveRefusesAnAnswerThePortDoesNotAllow(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		outcome WriteOutcome
		err     error
	}{
		"a commit with nothing to record": {outcome: WriteOutcome{Committed: true}},
		"neither an outcome nor a reason": {outcome: WriteOutcome{}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app, files := saving(t, sample(t))
			files.outcome, files.writeErr = tc.outcome, tc.err

			editValue(t, app, "/server/ports/0", "8081")
			press(app, ActionSave{})

			if !app.doc.IsDirty() {
				t.Error("the document was marked saved on an answer the port does not allow")
			}

			if app.Status().Error == "" {
				t.Error("the store's answer was accepted silently")
			}
		})
	}
}

// Saving is not an edit. What the reader can undo, where they are standing
// and how they are looking at the document are all untouched by it.
func TestSaveLeavesTheSessionWhereItWas(t *testing.T) {
	t.Parallel()

	app, _ := saving(t, sample(t))

	editValue(t, app, "/server/ports/0", "8081")
	press(app, ActionToggleView{})
	standOn(t, app, "/server")
	press(app, ActionMoveOut{})

	before := struct {
		versions int
		cursor   string
		view     ViewMode
		scroll   int
		folded   int
	}{
		versions: len(app.history.entries),
		cursor:   cursorOf(app),
		view:     app.view.ViewMode,
		scroll:   app.view.Scroll,
		folded:   len(app.view.Collapsed),
	}

	press(app, ActionSave{})

	if got := len(app.history.entries); got != before.versions {
		t.Errorf("history holds %d versions after saving, want %d", got, before.versions)
	}

	if got := cursorOf(app); got != before.cursor {
		t.Errorf("the cursor moved to %q, want %q", got, before.cursor)
	}

	if got := app.view.ViewMode; got != before.view {
		t.Errorf("the view changed to %v, want %v", got, before.view)
	}

	if got := app.view.Scroll; got != before.scroll {
		t.Errorf("the window moved to %d, want %d", got, before.scroll)
	}

	if got := len(app.view.Collapsed); got != before.folded {
		t.Errorf("%d nodes are folded after saving, want %d", got, before.folded)
	}
}

// Saving records which tree is on disk and nothing else, so undoing past it
// makes the document unsaved again and coming forward clears it.
func TestSaveMovesOnlyWhichVersionCountsAsSaved(t *testing.T) {
	t.Parallel()

	app, _ := saving(t, sample(t))
	editValue(t, app, "/server/ports/0", "8081")

	press(app, ActionSave{})

	press(app, ActionUndo{})

	if !app.doc.IsDirty() {
		t.Error("undoing past the saved version left the document clean")
	}

	press(app, ActionRedo{})

	if app.doc.IsDirty() {
		t.Error("returning to the saved version left the document dirty")
	}
}

// The version being saved is whichever one is current, including one reached
// by undoing.
func TestSaveWritesTheVersionThatIsCurrent(t *testing.T) {
	t.Parallel()

	app, files := saving(t, sample(t))

	editValue(t, app, "/server/ports/0", "8081")
	editValue(t, app, "/server/host", "127.0.0.1")

	press(app, ActionUndo{})

	want := app.doc.Root()

	press(app, ActionSave{})

	if got := string(files.written[0]); got != string(domain.Encode(want, app.format)) {
		t.Error("the version written was not the one on screen")
	}

	if app.doc.IsDirty() {
		t.Error("the document is dirty after the version on screen was written")
	}

	press(app, ActionRedo{})

	if !app.doc.IsDirty() {
		t.Error("redoing to a version that was never written left the document clean")
	}
}
