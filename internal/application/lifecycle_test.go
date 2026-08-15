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

			if app.Status().Error == "" {
				t.Error("the failure was not put on screen")
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

	if app.Status().Error == "" {
		t.Error("the failure was not put on screen")
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
			if info.Kind != PromptChoice || info.Title != writeErr.Error() {
				t.Errorf("the prompt reads %q, want the reason the save failed", info.Title)
			}

			if got, want := promptKeys(info), []rune{'o'}; string(got) != string(want) {
				t.Errorf("the prompt offers %q, want %q", string(got), string(want))
			}

			if app.Status().Error != writeErr.Error() {
				t.Error("the bar does not carry the failure while the dialog is up")
			}

			press(app, dismiss)

			if app.Mode() != ModeNormal {
				t.Errorf("mode = %v, want the message gone", app.Mode())
			}

			if app.Status().Error != "" {
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
