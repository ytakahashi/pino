package application

import (
	"errors"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

func TestAnUntouchedStdinDocumentQuitsWithoutSaving(t *testing.T) {
	t.Parallel()

	app, files := openingStdin(t, sample(t), "")

	if !quits(app.Do(ActionQuit{})) {
		t.Error("an untouched stdin document did not quit at once")
	}

	if len(files.checks) != 0 || len(files.writes) != 0 {
		t.Errorf("quitting checked %v and wrote %v, want neither", files.checks, files.writes)
	}
}

func TestReloadDoesNothingUntilAStdinDocumentHasBeenSaved(t *testing.T) {
	t.Parallel()

	app, files := openingStdin(t, sample(t), "")
	before := app.doc.Root()

	press(app, ActionReload{})

	if app.doc.Root() != before || len(files.reads) != 0 {
		t.Errorf("reload changed stdin or read %v before it had a file", files.reads)
	}
}

func TestSavingStdinToARequestedPathWritesEvenWhileClean(t *testing.T) {
	t.Parallel()

	app, files := openingStdin(t, sample(t), "out.json")

	press(app, ActionSave{})

	if len(files.checks) != 1 || files.checks[0] != "out.json" {
		t.Fatalf("checks = %v, want out.json once", files.checks)
	}

	if files.checked[0] != nil {
		t.Errorf("expected Meta = %#v, want nil", files.checked[0])
	}

	if len(files.writes) != 1 || files.writes[0] != "out.json" {
		t.Fatalf("writes = %v, want out.json once", files.writes)
	}

	status := app.Status()
	if status.Name != "out.json" || status.New || status.Dirty {
		t.Errorf("status after save = %+v, want a clean existing out.json", status)
	}

	if src, ok := app.source.(FileSource); !ok || src.Path != "out.json" || src.New {
		t.Errorf("source after save = %#v, want out.json", app.source)
	}

	press(app, ActionSave{})
	if len(files.writes) != 1 {
		t.Errorf("an unchanged saved document was written %d times, want once", len(files.writes))
	}

	files.data["out.json"] = []byte(testSource)
	press(app, ActionReload{})
	if got := files.reads[len(files.reads)-1]; got != "out.json" {
		t.Errorf("reload read %q, want out.json", got)
	}
}

func TestSavingMinifiedStdinAddsAFinalNewline(t *testing.T) {
	t.Parallel()

	root := testTree(t)
	files := &fakeFileStore{
		outcome: WriteOutcome{Meta: writtenMeta, Committed: true},
	}
	parser := &fakeParser{root: root}
	app := New(Deps{Parser: parser, Files: files}, Config{})

	if err := app.OpenStdin([]byte(`{"a":1}`), "out.json"); err != nil {
		t.Fatalf("OpenStdin: %v", err)
	}
	parser.parse = func([]byte, domain.Dialect) (domain.Node, error) { return app.doc.Root(), nil }

	press(app, ActionSave{})

	if got, want := string(files.written[0]), "{\n  \"a\": 1\n}\n"; got != want {
		t.Errorf("written bytes = %q, want %q", got, want)
	}
}

func TestSavingStdinAsksBeforeReplacingAnOccupiedPath(t *testing.T) {
	t.Parallel()

	app, files := openingStdin(t, sample(t), "out.json")
	files.status = ChangeModified

	press(app, ActionSave{})

	if len(files.writes) != 0 {
		t.Fatal("an occupied output path was overwritten without asking")
	}

	if got := app.Prompt(); got.Title != "out.json already exists." || string(promptKeys(got)) != "oc" {
		t.Errorf("prompt = %+v, want Overwrite and Cancel", got)
	}

	if effects := app.Do(ActionPromptChoose{Key: 'o'}); len(effects) != 0 {
		t.Errorf("overwrite effects = %v, want none", effects)
	}

	if len(files.writes) != 1 {
		t.Errorf("overwrite wrote %d times, want once", len(files.writes))
	}
}

func TestCancellingStdinOverwriteWritesNothing(t *testing.T) {
	t.Parallel()

	for _, cancel := range []Action{ActionPromptChoose{Key: 'c'}, ActionCancel{}} {
		t.Run(describeAction(cancel), func(t *testing.T) {
			t.Parallel()

			app, files := openingStdin(t, sample(t), "out.json")
			files.status = ChangeModified
			press(app, ActionSave{}, cancel)

			if app.Mode() != ModeNormal || len(files.writes) != 0 {
				t.Errorf("mode = %v, writes = %v; want normal and no writes", app.Mode(), files.writes)
			}
		})
	}
}

func TestCancellingSaveAsOverwriteAsksForANameAgain(t *testing.T) {
	t.Parallel()

	app, files := openingStdin(t, sample(t), "")
	files.status = ChangeModified

	press(app, ActionSave{}, ActionPromptSubmit{Text: "out.json"}, ActionPromptChoose{Key: 'c'})

	if app.Mode() != ModeNormal || len(files.writes) != 0 {
		t.Fatalf("mode = %v, writes = %v; want normal and no writes", app.Mode(), files.writes)
	}

	beginInput(t, app.Do(ActionSave{}))
	if got := app.Prompt(); got.Kind != PromptText || got.Title != "Save as" {
		t.Errorf("next prompt = %+v, want Save as", got)
	}
}

func TestStdinSaveRefusesAChangeStateThatCannotFollowANilMeta(t *testing.T) {
	t.Parallel()

	app, files := openingStdin(t, sample(t), "out.json")
	files.status = ChangeDeleted

	press(app, ActionSave{})

	if len(files.writes) != 0 {
		t.Error("stdin was written after an undefined change result")
	}

	if got := app.Status().Notice; got == nil || got.Detail != errStoreStatus.Error() {
		t.Errorf("notice = %+v, want the undefined store status", got)
	}
}

func TestANewFileKeepsTheReloadableConflictPrompt(t *testing.T) {
	t.Parallel()

	app, files := creating(t)
	files.status = ChangeModified

	press(app, ActionSave{})

	if got := string(promptKeys(app.Prompt())); got != "roc" {
		t.Errorf("prompt keys = %q, want the existing Reload, Overwrite and Cancel", got)
	}
}

func TestFailedStdinWriteDoesNotAcquireTheOutputPath(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("permission denied")
	app, files := openingStdin(t, sample(t), "out.json")
	files.outcome, files.writeErr = WriteOutcome{}, writeErr
	editValue(t, app, "/server/ports/0", "8081")

	press(app, ActionSave{})

	if src, ok := app.source.(StdinSource); !ok || src.Out != "out.json" {
		t.Errorf("source = %#v, want the original stdin destination", app.source)
	}

	if !app.doc.IsDirty() {
		t.Error("the stdin document was marked saved after a failed write")
	}

	if got := app.Status().Notice; got == nil || got.Detail != writeErr.Error() {
		t.Errorf("notice = %+v, want the write failure", got)
	}
}

func TestCommittedStdinWriteAcquiresThePathEvenWhenDurabilityFails(t *testing.T) {
	t.Parallel()

	syncErr := errors.New("the directory could not be synced")
	app, files := openingStdin(t, sample(t), "out.json")
	files.writeErr = syncErr

	press(app, ActionSave{})

	if src, ok := app.source.(FileSource); !ok || src.Path != "out.json" {
		t.Errorf("source = %#v, want the committed output file", app.source)
	}

	if app.doc.IsDirty() {
		t.Error("the stdin document is dirty although its write committed")
	}

	if got := app.Status().Notice; got == nil || got.Detail != syncErr.Error() || got.Severity != NoticeWarning {
		t.Errorf("notice = %+v, want the durability warning", got)
	}
}

func TestSavingStdinWithoutAPathAsksForOne(t *testing.T) {
	t.Parallel()

	app, files := openingStdin(t, sample(t), "")

	effects := app.Do(ActionSave{})
	beginInput(t, effects)

	if got := app.Prompt(); got.Kind != PromptText || got.Title != "Save as" || got.Multiline {
		t.Errorf("prompt = %+v, want a one-line Save as prompt", got)
	}

	if got := app.Do(ActionPromptSubmit{}); len(got) != 0 {
		t.Errorf("empty submit effects = %v, want none", got)
	}

	if app.Prompt().Error != errFileNameRequired {
		t.Errorf("error = %q, want %q", app.Prompt().Error, errFileNameRequired)
	}

	press(app, ActionPromptChange{Text: "out.json"})
	if app.Prompt().Error != "" {
		t.Errorf("error = %q after valid input, want it cleared", app.Prompt().Error)
	}

	if len(files.checks) != 0 || len(files.writes) != 0 {
		t.Errorf("empty path checked %v and wrote %v", files.checks, files.writes)
	}

	if effects := app.Do(ActionPromptSubmit{Text: " out.json "}); len(effects) != 0 {
		t.Errorf("save effects = %v, want none", effects)
	}

	if len(files.writes) != 1 || files.writes[0] != " out.json " {
		t.Errorf("writes = %v, want the untrimmed path", files.writes)
	}
}

func TestCancellingSaveAsAsksAgainOnTheNextSave(t *testing.T) {
	t.Parallel()

	app, files := openingStdin(t, sample(t), "")
	press(app, ActionSave{}, ActionCancel{})

	if app.Mode() != ModeNormal || len(files.writes) != 0 {
		t.Fatalf("mode = %v, writes = %v; want normal and no writes", app.Mode(), files.writes)
	}

	beginInput(t, app.Do(ActionSave{}))
	if got := app.Prompt(); got.Kind != PromptText || got.Title != "Save as" {
		t.Errorf("next prompt = %+v, want Save as", got)
	}
}

func TestFailedSaveAsForgetsTheUnwrittenPath(t *testing.T) {
	t.Parallel()

	app, files := openingStdin(t, sample(t), "")
	writeErr := errors.New("permission denied")
	files.outcome, files.writeErr = WriteOutcome{}, writeErr

	press(app, ActionSave{}, ActionPromptSubmit{Text: "bad/out.json"})

	if got := app.Status().Notice; got == nil || got.Detail != writeErr.Error() {
		t.Fatalf("notice = %+v, want the write failure", got)
	}

	press(app, ActionPromptChoose{Key: 'o'})

	if src, ok := app.source.(StdinSource); !ok || src.Out != "" {
		t.Errorf("source = %#v, want stdin with no destination", app.source)
	}

	beginInput(t, app.Do(ActionSave{}))
	if got := app.Prompt(); got.Kind != PromptText || got.Title != "Save as" {
		t.Errorf("next prompt = %+v, want Save as", got)
	}
}

func TestSaveAsAndQuitLeavesOnlyAfterACommit(t *testing.T) {
	t.Parallel()

	app, files := openingStdin(t, sample(t), "")
	editValue(t, app, "/server/ports/0", "8081")

	press(app, ActionQuit{})
	beginInput(t, app.Do(ActionPromptChoose{Key: 's'}))

	if !quits(app.Do(ActionPromptSubmit{Text: "out.json"})) {
		t.Error("save-as on the way out did not quit after committing")
	}

	if len(files.writes) != 1 || app.doc.IsDirty() {
		t.Errorf("writes = %v, dirty = %v; want one saved document", files.writes, app.doc.IsDirty())
	}
}

func TestSaveAsAndQuitCarriesLeavingThroughOverwrite(t *testing.T) {
	t.Parallel()

	app, files := openingStdin(t, sample(t), "")
	files.status = ChangeModified
	editValue(t, app, "/server/ports/0", "8081")

	press(app, ActionQuit{})
	beginInput(t, app.Do(ActionPromptChoose{Key: 's'}))

	if quits(app.Do(ActionPromptSubmit{Text: "out.json"})) {
		t.Fatal("pino quit before the occupied path was confirmed")
	}

	if !quits(app.Do(ActionPromptChoose{Key: 'o'})) {
		t.Error("pino did not quit after the confirmed overwrite committed")
	}
}
