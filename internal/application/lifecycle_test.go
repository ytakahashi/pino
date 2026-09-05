package application

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// The flows here are the ones that stand between an edited document and the
// file it came from. What they are checked on is that nothing is written, and
// nothing is thrown away, except where the reader said so.

// conflicted is a session whose save was stopped by the file having changed.
func conflicted(t *testing.T, status ChangeStatus) (*App, *fakeFileStore) {
	t.Helper()

	app, files := saving(t, sample(t))
	files.status = status

	editValue(t, app, "/server/ports/0", "8081")
	press(app, ActionSave{})

	if app.Mode() != ModeConfirm {
		t.Fatalf("mode = %v after a save met a changed file, want %v", app.Mode(), ModeConfirm)
	}

	return app, files
}

// reloadTo makes the file read as root the next time it is read.
func reloadTo(t *testing.T, a *App, root domain.Node) {
	t.Helper()

	parserOf(t, a).parse = func([]byte, domain.Dialect) (domain.Node, error) { return root, nil }
}

// quits reports whether an action asked the program to stop.
func quits(effects []Effect) bool {
	for _, e := range effects {
		if _, ok := e.(EffectQuit); ok {
			return true
		}
	}

	return false
}

// Leaving with nothing to lose is leaving. A document nobody typed into is
// one of those, including one whose file has still to be created: there is
// nothing to write, and writing it would create a file nobody asked for.
func TestQuittingLeavesAtOnceWhenThereIsNothingToSave(t *testing.T) {
	t.Parallel()

	tests := map[string]func(t *testing.T) (*App, *fakeFileStore){
		"no document at all": func(t *testing.T) (*App, *fakeFileStore) {
			t.Helper()

			files := &fakeFileStore{data: map[string][]byte{}}

			return New(Deps{
				Parser:   &fakeParser{},
				Files:    files,
				JSONView: &fakeRenderer{},
				TreeView: &fakeRenderer{},
			}, Config{}), files
		},
		"a document as it was read": func(t *testing.T) (*App, *fakeFileStore) {
			t.Helper()

			return saving(t, sample(t))
		},
		"a new document nobody typed into": func(t *testing.T) (*App, *fakeFileStore) {
			t.Helper()

			return creating(t)
		},
		"a document edited back to what it was": func(t *testing.T) (*App, *fakeFileStore) {
			t.Helper()

			app, files := saving(t, sample(t))
			editValue(t, app, "/server/ports/0", "8081")
			press(app, ActionUndo{})

			return app, files
		},
	}

	for name, open := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app, files := open(t)

			if !quits(app.Do(ActionQuit{})) {
				t.Error("pino stayed open with nothing to save")
			}

			if app.Mode() != ModeNormal {
				t.Errorf("mode = %v, want no question asked", app.Mode())
			}

			if len(files.writes) != 0 {
				t.Error("leaving wrote a file")
			}
		})
	}
}

// Leaving with unsaved work in the document is a question. The three answers
// are on the prompt, and nothing else there does anything.
func TestQuittingADirtyDocumentAsks(t *testing.T) {
	t.Parallel()

	app, files := saving(t, sample(t))
	editValue(t, app, "/server/ports/0", "8081")

	if quits(app.Do(ActionQuit{})) {
		t.Fatal("pino left with unsaved changes in the document")
	}

	info := app.Prompt()
	if info.Kind != PromptChoice || info.Title != "You have unsaved changes." {
		t.Errorf("the prompt reads %q, want the unsaved changes to be named", info.Title)
	}

	if got, want := promptKeys(info), []rune{'s', 'd', 'c'}; string(got) != string(want) {
		t.Errorf("the prompt offers %q, want %q", string(got), string(want))
	}

	for _, key := range []rune{'q', 'y', 'n', 'x'} {
		if quits(app.Do(ActionPromptChoose{Key: key})) {
			t.Fatalf("%q was taken as an answer and left", key)
		}
	}

	if len(files.writes) != 0 {
		t.Error("a key the prompt does not offer wrote the document")
	}
}

// Asking again is asking again. Pressing the key that leaves twice is not a
// way of saying "discard", which is a key of its own that has to be read.
func TestAskingToQuitAgainKeepsTheQuestion(t *testing.T) {
	t.Parallel()

	app, files := saving(t, sample(t))
	editValue(t, app, "/server/ports/0", "8081")

	for range 3 {
		if quits(app.Do(ActionQuit{})) {
			t.Fatal("pino left after being asked repeatedly, with the changes unsaved")
		}
	}

	if app.Prompt().Title != "You have unsaved changes." {
		t.Errorf("the prompt reads %q, want the question still standing", app.Prompt().Title)
	}

	if !app.doc.IsDirty() {
		t.Error("the document was marked saved by being asked to quit")
	}

	if len(files.writes) != 0 {
		t.Error("asking to quit wrote the document")
	}
}

// Discarding is the one answer that leaves work behind, and it writes
// nothing: the file is as it was.
func TestDiscardingLeavesWithoutWriting(t *testing.T) {
	t.Parallel()

	app, files := saving(t, sample(t))
	editValue(t, app, "/server/ports/0", "8081")

	press(app, ActionQuit{})

	if !quits(app.Do(ActionPromptChoose{Key: 'd'})) {
		t.Error("discarding did not leave")
	}

	if len(files.writes) != 0 {
		t.Error("discarding wrote the document")
	}
}

// Cancelling puts the reader back where they were, with everything in it.
func TestCancellingTheQuestionLeavesTheSessionAsItWas(t *testing.T) {
	t.Parallel()

	for _, cancel := range []Action{ActionPromptChoose{Key: 'c'}, ActionCancel{}} {
		t.Run(describeAction(cancel), func(t *testing.T) {
			t.Parallel()

			app, files := saving(t, sample(t))
			editValue(t, app, "/server/ports/0", "8081")

			root, versions, cursor := app.doc.Root(), len(app.history.entries), cursorOf(app)

			press(app, ActionQuit{})

			if quits(app.Do(cancel)) {
				t.Fatal("cancelling left")
			}

			if app.Mode() != ModeNormal {
				t.Errorf("mode = %v, want the question gone", app.Mode())
			}

			if app.doc.Root() != root || len(app.history.entries) != versions || cursorOf(app) != cursor {
				t.Error("cancelling changed the document or where the reader was standing")
			}

			if len(files.writes) != 0 {
				t.Error("cancelling wrote the document")
			}
		})
	}
}

// Saving on the way out leaves only once the document has reached the file.
func TestSaveAndQuitLeavesOnlyOnceTheDocumentIsWritten(t *testing.T) {
	t.Parallel()

	app, files := saving(t, sample(t))
	editValue(t, app, "/server/ports/0", "8081")

	press(app, ActionQuit{})

	if !quits(app.Do(ActionPromptChoose{Key: 's'})) {
		t.Error("saving on the way out did not leave although the write committed")
	}

	if len(files.writes) != 1 {
		t.Errorf("the store was asked to write %d times, want once", len(files.writes))
	}

	if app.doc.IsDirty() {
		t.Error("the document is dirty after being written")
	}
}

// Every way a save can fail to reach the file keeps pino open, with the
// document still in it and the reason on screen.
func TestSaveAndQuitStaysWhenTheDocumentDidNotReachTheFile(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("permission denied")

	tests := map[string]func(t *testing.T, a *App, f *fakeFileStore){
		"the file changed underneath": func(_ *testing.T, _ *App, f *fakeFileStore) {
			f.status = ChangeModified
		},
		"the write failed": func(_ *testing.T, _ *App, f *fakeFileStore) {
			f.outcome, f.writeErr = WriteOutcome{}, writeErr
		},
		"the document would not survive encoding": func(t *testing.T, a *App, _ *fakeFileStore) {
			t.Helper()

			parserOf(t, a).parse = func([]byte, domain.Dialect) (domain.Node, error) {
				return nil, writeErr
			}
		},
	}

	for name, arrange := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app, files := saving(t, sample(t))
			editValue(t, app, "/server/ports/0", "8081")

			arrange(t, app, files)

			press(app, ActionQuit{})

			if quits(app.Do(ActionPromptChoose{Key: 's'})) {
				t.Error("pino left although the document had not reached the file")
			}

			if !app.doc.IsDirty() {
				t.Error("the document was marked saved although it was not written")
			}

			if app.Mode() != ModeConfirm {
				t.Errorf("mode = %v, want what happened to be on screen", app.Mode())
			}
		})
	}
}

// The rename committed and something after it did not. The document is saved,
// so there is nothing left to lose by staying — and staying is what keeps the
// report of it from leaving the screen with the program.
func TestSaveAndQuitStaysToReportAnUnconfirmedWrite(t *testing.T) {
	t.Parallel()

	syncErr := errors.New("the directory could not be synced")

	app, files := saving(t, sample(t))
	files.writeErr = syncErr

	editValue(t, app, "/server/ports/0", "8081")

	press(app, ActionQuit{})

	if quits(app.Do(ActionPromptChoose{Key: 's'})) {
		t.Error("pino left with a failure nobody could have read")
	}

	if app.doc.IsDirty() {
		t.Error("the document is dirty although the file was replaced")
	}

	if got := app.Status().Notice; got == nil || got.Detail != syncErr.Error() || got.Severity != NoticeWarning {
		t.Errorf("the notice = %+v, want a durability warning", got)
	}

	// The document is saved, so asking again goes straight out.
	press(app, ActionPromptChoose{Key: 'o'})

	if !quits(app.Do(ActionQuit{})) {
		t.Error("pino asked again although the document was saved")
	}
}

// A save on the way out that met a changed file carries the leaving with it:
// overwriting ends the session, and reloading is choosing to stay and look at
// the other document.
func TestAConflictOnTheWayOutLeavesOnlyWhenOverwritten(t *testing.T) {
	t.Parallel()

	t.Run("overwriting", func(t *testing.T) {
		t.Parallel()

		app, files := saving(t, sample(t))
		files.status = ChangeModified

		editValue(t, app, "/server/ports/0", "8081")

		press(app, ActionQuit{}, ActionPromptChoose{Key: 's'})

		if !quits(app.Do(ActionPromptChoose{Key: 'o'})) {
			t.Error("overwriting on the way out did not leave")
		}

		if len(files.writes) != 1 {
			t.Errorf("the store was asked to write %d times, want once", len(files.writes))
		}
	})

	t.Run("reloading", func(t *testing.T) {
		t.Parallel()

		app, files := saving(t, sample(t))
		files.status = ChangeModified

		editValue(t, app, "/server/ports/0", "8081")

		press(app, ActionQuit{}, ActionPromptChoose{Key: 's'})

		reloadTo(t, app, testdocsOutside(t))

		if quits(app.Do(ActionPromptChoose{Key: 'r'})) {
			t.Error("reloading on the way out left, taking the document nobody had looked at with it")
		}

		if app.Mode() != ModeNormal {
			t.Errorf("mode = %v after reloading, want the document on screen", app.Mode())
		}

		if len(files.writes) != 0 {
			t.Error("reloading wrote the document")
		}
	})
}

// Text arrives from a widget only a text prompt asks for. It cannot reach the
// question about leaving through the terminal, and must do nothing when it is
// driven straight at this layer.
func TestTextActionsDoNothingToTheQuestionAboutLeaving(t *testing.T) {
	t.Parallel()

	app, files := saving(t, sample(t))
	editValue(t, app, "/server/ports/0", "8081")

	press(app, ActionQuit{})

	root := app.doc.Root()

	press(app,
		ActionPromptChange{Text: "typed"},
		ActionPromptSubmit{Text: "typed"},
	)

	if app.Mode() != ModeConfirm {
		t.Errorf("mode = %v, want the question still standing", app.Mode())
	}

	if app.doc.Root() != root {
		t.Error("text answered a question the prompt was not asking")
	}

	if len(files.writes) != 0 {
		t.Error("text answering the question wrote the document")
	}
}

// Cancel is for a reader who wants to look at what happened before deciding.
// Nothing at all may come of it.
func TestCancellingAConflictLeavesEverythingAsItWas(t *testing.T) {
	t.Parallel()

	for _, cancel := range []Action{ActionPromptChoose{Key: 'c'}, ActionCancel{}} {
		t.Run(describeAction(cancel), func(t *testing.T) {
			t.Parallel()

			app, files := conflicted(t, ChangeModified)
			before := app.doc.Root()

			press(app, cancel)

			if app.Mode() != ModeNormal {
				t.Errorf("mode = %v, want the prompt gone", app.Mode())
			}

			if app.doc.Root() != before {
				t.Error("the document was replaced by cancelling")
			}

			if !app.doc.IsDirty() {
				t.Error("cancelling marked the document saved")
			}

			if len(files.writes) != 0 {
				t.Error("cancelling wrote the document")
			}
		})
	}
}

// Overwriting is the reader saying they know. The check is skipped once, and
// the next save asks again.
func TestOverwritingSkipsTheCheckExactlyOnce(t *testing.T) {
	t.Parallel()

	app, files := conflicted(t, ChangeModified)

	pick(app, 'o')

	if len(files.writes) != 1 {
		t.Fatalf("the store was asked to write %d times, want once", len(files.writes))
	}

	if app.doc.IsDirty() {
		t.Error("the document is dirty after being overwritten")
	}

	if app.Mode() != ModeNormal {
		t.Errorf("mode = %v after overwriting, want the prompt gone", app.Mode())
	}

	checks := len(files.checks)

	editValue(t, app, "/server/host", "127.0.0.1")
	press(app, ActionSave{})

	if len(files.checks) != checks+1 {
		t.Error("the save after an overwrite did not ask about the file again")
	}

	if app.Mode() != ModeConfirm {
		t.Error("the save after an overwrite went through although the file still reads as changed")
	}
}

// What is written is made again from the document as it stands, so the bytes
// cannot be from a version the reader has since moved away from.
func TestOverwritingWritesTheDocumentAsItStands(t *testing.T) {
	t.Parallel()

	app, files := conflicted(t, ChangeModified)

	pick(app, 'o')

	if want := string(domain.Encode(app.doc.Root(), app.format)); string(files.written[0]) != want {
		t.Errorf("the store was given %q, want %q", files.written[0], want)
	}
}

// A reload requested with no unsaved work can take the file immediately.
func TestRequestingReloadOfACleanDocumentReadsItAtOnce(t *testing.T) {
	t.Parallel()

	app, files := saving(t, sample(t))
	outside := testdocsOutside(t)
	reloadTo(t, app, outside)

	reads := len(files.reads)
	press(app, ActionReload{})

	if len(files.reads) != reads+1 {
		t.Errorf("the store was read %d times after requesting reload, want once", len(files.reads)-reads)
	}

	if app.doc.Root() != outside {
		t.Error("the document is not the one the file now holds")
	}

	if app.Mode() != ModeNormal {
		t.Errorf("mode = %v after reloading a clean document, want no question", app.Mode())
	}
}

// Unsaved work is not thrown away until the reader chooses the discard
// answer shown on the prompt. The file is not read while that answer is
// pending, because even observing it is unnecessary if the reader cancels.
func TestRequestingReloadOfADirtyDocumentAsksBeforeReading(t *testing.T) {
	t.Parallel()

	app, files := saving(t, sample(t))
	editValue(t, app, "/server/ports/0", "8081")

	before, reads := app.doc.Root(), len(files.reads)
	press(app, ActionReload{})

	info := app.Prompt()
	if info.Kind != PromptChoice || info.Title != "Reloading will discard your unsaved changes." {
		t.Errorf("the prompt = %+v, want the reload warning", info)
	}

	want := []Choice{
		{Key: 'd', Label: "Discard changes and reload"},
		{Key: 'c', Label: "Cancel"},
	}
	if len(info.Choices) != len(want) {
		t.Fatalf("the prompt offers %+v, want %+v", info.Choices, want)
	}
	for i, choice := range info.Choices {
		if choice != want[i] {
			t.Errorf("choice %d = %+v, want %+v", i, choice, want[i])
		}
	}

	if app.doc.Root() != before || !app.doc.IsDirty() {
		t.Error("asking to reload changed or marked the document saved")
	}

	if len(files.reads) != reads {
		t.Error("asking to reload read the file before the reader chose to discard")
	}
}

// Discarding gives the file the same path through reload as the choice on a
// save conflict, so the replacement starts as a clean document with no old
// versions behind it.
func TestDiscardingChangesReloadsTheDocument(t *testing.T) {
	t.Parallel()

	app, _ := saving(t, sample(t))
	editValue(t, app, "/server/ports/0", "8081")

	outside := testdocsOutside(t)
	reloadTo(t, app, outside)

	press(app, ActionReload{}, ActionPromptChoose{Key: 'd'})

	if app.doc.Root() != outside {
		t.Error("discarding did not show what the file now holds")
	}

	if app.doc.IsDirty() {
		t.Error("the freshly reloaded document is dirty")
	}

	if got := len(app.history.entries); got != 1 {
		t.Errorf("history holds %d versions after reloading, want 1", got)
	}

	if app.Mode() != ModeNormal {
		t.Errorf("mode = %v after reloading, want the document on screen", app.Mode())
	}
}

// Cancelling a requested reload leaves both the document and its view alone,
// and never consults the file that the reader chose not to take.
func TestCancellingRequestedReloadLeavesTheSessionAsItWas(t *testing.T) {
	t.Parallel()

	for _, cancel := range []Action{ActionPromptChoose{Key: 'c'}, ActionCancel{}} {
		t.Run(describeAction(cancel), func(t *testing.T) {
			t.Parallel()

			app, files := saving(t, sample(t))
			editValue(t, app, "/server/ports/0", "8081")
			app.view.Collapse(pointer(t, "/server/ports"))

			before := struct {
				root     domain.Node
				versions int
				cursor   string
				scroll   int
				format   domain.Format
				meta     Meta
				reads    int
			}{
				root:     app.doc.Root(),
				versions: len(app.history.entries),
				cursor:   cursorOf(app),
				scroll:   app.view.Scroll,
				format:   app.format,
				meta:     app.meta,
				reads:    len(files.reads),
			}

			press(app, ActionReload{}, cancel)

			if app.Mode() != ModeNormal {
				t.Errorf("mode = %v, want the question gone", app.Mode())
			}

			if app.doc.Root() != before.root || len(app.history.entries) != before.versions {
				t.Error("cancelling changed the document or its history")
			}

			if cursorOf(app) != before.cursor || app.view.Scroll != before.scroll ||
				!app.view.IsCollapsed(pointer(t, "/server/ports")) {
				t.Error("cancelling changed how the document is being viewed")
			}

			if app.format != before.format || app.meta != before.meta {
				t.Error("cancelling changed the layout or the Meta")
			}

			if len(files.reads) != before.reads {
				t.Error("cancelling read the file")
			}
		})
	}
}

// A separately parsed tree can describe the same document. Reload still
// installs it to refresh file-derived state, but there is no changed content
// for which the reader's position or folds need to be discarded.
func TestReloadingAnEqualDocumentKeepsHowItIsViewed(t *testing.T) {
	t.Parallel()

	app, files := saving(t, sample(t))
	press(app, ActionResize{Height: 4}, ActionToggleView{})
	acceptSearch(t, app, "pino")
	app.view.Collapse(pointer(t, "/server/ports"))
	standOn(t, app, "/debug")

	before := struct {
		root      domain.Node
		cursor    string
		scroll    int
		view      ViewMode
		height    int
		maxStrLen int
		search    SearchInfo
	}{
		root:      app.doc.Root(),
		cursor:    cursorOf(app),
		scroll:    app.view.Scroll,
		view:      app.view.ViewMode,
		height:    app.height,
		maxStrLen: app.view.MaxStrLen,
		search:    *app.Status().Search,
	}

	equal := sample(t)
	if equal == before.root || !domain.Equal(equal, before.root) {
		t.Fatal("the reload fixture is not a distinct, equal tree")
	}

	raw := []byte("{\r\n\t\"layout\": true\r\n}\r\n")
	files.data[savePath] = raw
	files.meta = writtenMeta
	reloadTo(t, app, equal)

	press(app, ActionReload{})

	if app.doc.Root() != equal {
		t.Error("reload did not install the separately parsed tree")
	}

	if app.format != domain.DetectFormat(raw) || app.meta != Meta(writtenMeta) {
		t.Error("reload did not install the file's layout or Meta")
	}

	if cursorOf(app) != before.cursor || app.view.Scroll != before.scroll ||
		app.view.ViewMode != before.view || app.height != before.height ||
		app.view.MaxStrLen != before.maxStrLen || !app.view.IsCollapsed(pointer(t, "/server/ports")) {
		t.Error("reloading an equal document changed how it is being viewed")
	}

	if got := app.Status().Search; got == nil || *got != before.search {
		t.Errorf("search after reload = %+v, want %+v", got, before.search)
	}

	if got := len(app.history.entries); got != 1 {
		t.Errorf("history holds %d versions after reloading, want 1", got)
	}
}

// With no file-backed document there is nothing a reload request can read or
// ask about.
func TestRequestingReloadWithoutADocumentDoesNothing(t *testing.T) {
	t.Parallel()

	app := New(Deps{}, Config{})

	if effects := app.Do(ActionReload{}); effects != nil {
		t.Errorf("reload without a document returned effects %v", effects)
	}

	if app.Mode() != ModeNormal {
		t.Errorf("mode = %v after reload without a document, want normal", app.Mode())
	}
}

// Reloading is the other document winning. Everything the session knew about
// the old one goes, including the versions of it.
func TestReloadingShowsWhatTheFileNowHolds(t *testing.T) {
	t.Parallel()

	app, _ := conflicted(t, ChangeModified)

	press(app, ActionResize{Height: 12})
	press(app, ActionToggleView{})

	outside := testdocsOutside(t)
	reloadTo(t, app, outside)

	pick(app, 'r')

	if app.doc.Root() != outside {
		t.Error("the document is not the one the file now holds")
	}

	if app.doc.IsDirty() {
		t.Error("a freshly read document is dirty")
	}

	if app.Status().New {
		t.Error("a document read from a file is reported as new")
	}

	if got := len(app.history.entries); got != 1 {
		t.Errorf("history holds %d versions after reloading, want 1", got)
	}

	press(app, ActionUndo{})

	if app.doc.Root() != outside {
		t.Error("undoing after a reload reached a version of the document that was replaced")
	}

	if app.Mode() != ModeNormal {
		t.Errorf("mode = %v after reloading, want the prompt gone", app.Mode())
	}
}

// Where the reader was standing belongs to the document that has gone: an
// element added outside pino leaves the same pointer naming another value.
// How they are looking at it is the session's own and stays.
func TestReloadingKeepsTheViewAndForgetsThePosition(t *testing.T) {
	t.Parallel()

	app, _ := conflicted(t, ChangeModified)

	press(app, ActionResize{Height: 4})
	press(app, ActionToggleView{})
	standOn(t, app, "/server/ports/1")
	press(app, ActionMoveOut{})
	press(app, ActionScrollHalfDown{})

	view, height := app.view.ViewMode, app.height

	reloadTo(t, app, testdocsOutside(t))
	pick(app, 'r')

	if app.view.ViewMode != view {
		t.Errorf("the view changed to %v, want the %v the reader chose", app.view.ViewMode, view)
	}

	if app.height != height {
		t.Errorf("the height changed to %d, want %d", app.height, height)
	}

	if !app.view.Cursor.IsRoot() {
		t.Errorf("the cursor is at %q, want the root", cursorOf(app))
	}

	if app.view.Scroll != 0 {
		t.Errorf("the window is at %d, want the top", app.view.Scroll)
	}

	if len(app.view.Collapsed) != 0 {
		t.Errorf("%d nodes are folded, want none", len(app.view.Collapsed))
	}
}

// A new document whose path was taken by somebody else. Reloading opens what
// is there as an ordinary file, which is the one way a document stops being
// new without pino having written anything.
func TestReloadingANewDocumentOpensTheFileThatAppeared(t *testing.T) {
	t.Parallel()

	app, files := creating(t)
	files.status = ChangeModified

	press(app, ActionSave{})

	if app.Mode() != ModeConfirm {
		t.Fatalf("mode = %v after saving onto a path that was taken, want %v", app.Mode(), ModeConfirm)
	}

	appeared := testdocsOutside(t)
	files.data["new.json"] = []byte("{}")
	files.meta = openMeta

	reloadTo(t, app, appeared)

	pick(app, 'r')

	if app.doc.Root() != appeared {
		t.Error("the document is not the file that appeared at the path")
	}

	if app.Status().New {
		t.Error("the document is still reported as new after the file at its path was read")
	}

	if app.meta != Meta(openMeta) {
		t.Error("the session did not take the Meta of the file it read")
	}
}

// A file that cannot be read leaves the session exactly as it was. The
// document is read whole before any of it is installed, so there is no half
// reloaded state to be in.
func TestAFailedReloadChangesNothing(t *testing.T) {
	t.Parallel()

	readErr := errors.New("permission denied")

	tests := map[string]func(f *fakeFileStore, p *fakeParser){
		"the file cannot be read": func(f *fakeFileStore, _ *fakeParser) {
			f.err = readErr
		},
		"the file is no longer JSON": func(_ *fakeFileStore, p *fakeParser) {
			p.parse = func([]byte, domain.Dialect) (domain.Node, error) { return nil, readErr }
		},
	}

	for name, arrange := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app, files := conflicted(t, ChangeModified)

			standOn(t, app, "/server/host")

			before := struct {
				root     domain.Node
				versions int
				cursor   string
				format   domain.Format
				meta     Meta
			}{app.doc.Root(), len(app.history.entries), cursorOf(app), app.format, app.meta}

			arrange(files, parserOf(t, app))

			pick(app, 'r')

			if app.doc.Root() != before.root {
				t.Error("the document was replaced although it could not be read")
			}

			if !app.doc.IsDirty() {
				t.Error("the document was marked saved by a failed reload")
			}

			if got := len(app.history.entries); got != before.versions {
				t.Errorf("history holds %d versions, want %d", got, before.versions)
			}

			if got := cursorOf(app); got != before.cursor {
				t.Errorf("the cursor moved to %q, want %q", got, before.cursor)
			}

			if app.format != before.format || app.meta != before.meta {
				t.Error("the layout or the Meta was replaced by a failed reload")
			}

			if got := app.Status().Notice; got == nil || got.Summary != "Could not reload config.json." || got.Severity != NoticeError {
				t.Errorf("the notice = %+v, want a reload error", got)
			}
		})
	}
}

// A file that has been deleted has nothing to be read back. Turning that into
// the empty document a missing path opens as would throw away what the reader
// has been editing.
func TestReloadingADeletedFileKeepsTheDocument(t *testing.T) {
	t.Parallel()

	app, files := conflicted(t, ChangeDeleted)
	before := app.doc.Root()

	files.err = fs.ErrNotExist

	pick(app, 'r')

	if app.doc.Root() != before {
		t.Error("the document was replaced although the file is gone")
	}

	if app.Status().New {
		t.Error("the document became a new one when its file went")
	}

	if got := app.Status().Notice; got == nil || got.Summary != "Could not reload config.json." {
		t.Errorf("the notice = %+v, want a reload error", got)
	}
}

// A failure is held until it is acknowledged, and shows in both places: the
// dialog, which is answered and gone, and the bar, which is not.
func TestAFailureIsShownUntilItIsAcknowledged(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("permission denied")

	for _, dismiss := range []Action{ActionPromptChoose{Key: 'o'}, ActionCancel{}} {
		t.Run(describeAction(dismiss), func(t *testing.T) {
			t.Parallel()

			app, files := saving(t, sample(t))
			files.outcome, files.writeErr = WriteOutcome{}, writeErr

			editValue(t, app, "/server/ports/0", "8081")
			press(app, ActionSave{})

			info := app.Prompt()
			if info.Kind != PromptChoice || info.Notice == nil || info.Notice.Summary != "Could not save config.json." {
				t.Errorf("the prompt = %+v, want a save notice", info)
			}

			if got, want := promptKeys(info), []rune{'o'}; string(got) != string(want) {
				t.Errorf("the prompt offers %q, want %q", string(got), string(want))
			}

			if got := app.Status().Notice; got == nil || got.Detail != writeErr.Error() {
				t.Error("the bar does not carry the failure while the dialog is up")
			}

			press(app, dismiss)

			if app.Mode() != ModeNormal {
				t.Errorf("mode = %v, want the message gone", app.Mode())
			}

			if app.Status().Notice != nil {
				t.Error("the bar still carries a failure that was acknowledged")
			}

			// The document is still there to be saved again.
			if !app.doc.IsDirty() {
				t.Error("the document was marked saved by a failure being dismissed")
			}
		})
	}
}

// The keys a prompt draws are the keys it takes, and nothing else does
// anything. A key that did something undrawn would be a promise nobody made.
func TestALifecyclePromptTakesOnlyTheKeysItOffers(t *testing.T) {
	t.Parallel()

	app, files := conflicted(t, ChangeModified)

	for _, key := range []rune{'x', 'y', 's', 'q'} {
		pick(app, key)

		if app.Mode() != ModeConfirm {
			t.Fatalf("%q was taken as an answer to the conflict", key)
		}
	}

	if len(files.writes) != 0 {
		t.Error("a key the prompt does not offer wrote the document")
	}
}

// Text arrives from a widget that only a text prompt asks for. It cannot
// reach a lifecycle flow through the terminal, but an Action driven straight
// at this layer can, and must do nothing rather than reach into a flow that
// is not gathering text.
func TestTextActionsDoNothingToALifecycleFlow(t *testing.T) {
	t.Parallel()

	app, files := conflicted(t, ChangeModified)
	before := app.doc.Root()

	press(app,
		ActionPromptChange{Text: "typed"},
		ActionPromptSubmit{Text: "typed"},
	)

	if app.Mode() != ModeConfirm {
		t.Errorf("mode = %v, want the conflict still waiting", app.Mode())
	}

	if app.doc.Root() != before {
		t.Error("text answered a question the prompt was not asking")
	}

	if len(files.writes) != 0 {
		t.Error("text answering a conflict wrote the document")
	}
}
