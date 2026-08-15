package application

import (
	"slices"
	"testing"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// Editing a document from the outside: the keys that start an edit, the
// prompts they open, and what the document holds afterwards.
//
// Nothing here draws anything. What an edit produces is a tree, so what these
// check is the tree, the version behind it, and where the cursor was left.

func TestEnterOnAStringOffersTheWholeValue(t *testing.T) {
	t.Parallel()

	root, value := longString(t)
	app := session(t, root)
	standOn(t, app, "/motto")

	in := beginInput(t, app.Do(ActionEdit{}))

	// The row shows as much of the value as it has room for, and the prompt
	// has to start from the value itself: editing an abbreviation would save
	// the abbreviation.
	if row := app.Frame().Lines[1].Text(); len(row) >= len(value) {
		t.Fatalf("the row drew the value in full (%d bytes), so this proves nothing", len(row))
	}

	if in.Text != value {
		t.Errorf("the text box holds %d bytes, want the whole value (%d)", len(in.Text), len(value))
	}

	if !in.Multiline {
		t.Error("a string is being edited in a box that cannot hold a newline")
	}

	p := app.Prompt()
	if p.Kind != PromptText || p.Title != "Edit string" {
		t.Errorf("prompt = %v %q, want a text prompt titled %q", p.Kind, p.Title, "Edit string")
	}

	if app.Mode() != ModeEdit {
		t.Errorf("mode = %v, want %v", app.Mode(), ModeEdit)
	}
}

func TestEnterOnANumberOffersItsLiteral(t *testing.T) {
	t.Parallel()

	// The literal rather than the quantity: 1.50 and 1.5 are the same number
	// and not the same document.
	app := session(t, object(t, member("ratio", domain.NewNumber("1.50"))))
	standOn(t, app, "/ratio")

	in := beginInput(t, app.Do(ActionEdit{}))

	if in.Text != "1.50" {
		t.Errorf("the text box holds %q, want the literal as the document spells it", in.Text)
	}

	if in.Multiline {
		t.Error("a number is being edited in a box that can hold a newline")
	}

	if p := app.Prompt(); p.Title != "Edit number" || p.Multiline {
		t.Errorf("prompt = %q multiline=%v, want %q on one line", p.Title, p.Multiline, "Edit number")
	}
}

func TestEnterOnABooleanFlipsItStraightAway(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/debug")

	before := versions(app)

	if effects := app.Do(ActionEdit{}); len(effects) != 0 {
		t.Errorf("flipping a boolean asked for %d effects, want none", len(effects))
	}

	if got := nodeAt(t, app, "/debug").(*domain.Bool).Value(); !got {
		t.Error("the boolean was not flipped")
	}

	if app.Prompt().Kind != PromptNone || app.Mode() != ModeNormal {
		t.Error("flipping a boolean left a prompt open")
	}

	if got := versions(app); got != before+1 {
		t.Errorf("%d versions, want %d: one edit is one version", got, before+1)
	}

	if !app.doc.IsDirty() {
		t.Error("the document is not marked unsaved after an edit")
	}
}

func TestEnterOnANullAsksWhichTypeItShouldBe(t *testing.T) {
	t.Parallel()

	// A null holds nothing to type over, so the only edit it can take is
	// becoming something else.
	app := session(t, object(t, member("maybe", domain.NewNull())))
	standOn(t, app, "/maybe")
	app.Do(ActionEdit{})

	p := app.Prompt()
	if p.Kind != PromptChoice || p.Title != "type" {
		t.Errorf("prompt = %v %q, want the list of types", p.Kind, p.Title)
	}

	if got := keysOf(p); got != "snbzoa" {
		t.Errorf("keys = %q, want the six types", got)
	}
}

func TestEnterOnAContainerFoldsItAndUnfoldsIt(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/server")

	rows := len(app.Frame().Lines)

	app.Do(ActionEdit{})

	if !app.view.IsCollapsed(app.view.Cursor) {
		t.Error("the container did not fold")
	}

	if got := len(app.Frame().Lines); got >= rows {
		t.Errorf("%d rows after folding, want fewer than %d", got, rows)
	}

	app.Do(ActionEdit{})

	if app.view.IsCollapsed(app.view.Cursor) {
		t.Error("the container did not unfold")
	}

	if got := len(app.Frame().Lines); got != rows {
		t.Errorf("%d rows after unfolding, want the %d there were", got, rows)
	}

	if versions(app) != 1 {
		t.Error("folding was recorded as a version of the document")
	}
}

func TestEnterOnTheRootLeavesTheDocumentWhereItIs(t *testing.T) {
	t.Parallel()

	// The root has no folded form, so there is no toggle to make.
	app := session(t, sample(t))

	rows := len(app.Frame().Lines)
	app.Do(ActionEdit{})

	if len(app.view.Collapsed) != 0 {
		t.Errorf("the root was folded away: %v", foldsOf(app.view))
	}

	if got := len(app.Frame().Lines); got != rows {
		t.Errorf("%d rows, want the %d there were", got, rows)
	}
}

func TestEnterOnAnEmptyContainerDoesNothing(t *testing.T) {
	t.Parallel()

	// An empty container is drawn as neither open nor folded, so there is
	// nothing to toggle and nothing to hide.
	app := session(t, nested(t))
	standOn(t, app, "/empty")
	app.Do(ActionEdit{})

	if len(app.view.Collapsed) != 0 {
		t.Errorf("an empty container was folded: %v", foldsOf(app.view))
	}
}

func TestATypedValueReplacesTheOneThatWasThere(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/server/ports/0")
	app.Do(ActionEdit{})
	answer(app, "9090")

	if got := nodeAt(t, app, "/server/ports/0").(*domain.Number).Raw(); got != "9090" {
		t.Errorf("the value is %q, want %q", got, "9090")
	}

	if app.Mode() != ModeNormal || app.Prompt().Kind != PromptNone {
		t.Error("the prompt stayed open after the answer was taken")
	}

	if got := cursorOf(app); got != "/server/ports/0" {
		t.Errorf("cursor at %q, want the node that was edited", got)
	}

	if versions(app) != 2 {
		t.Errorf("%d versions, want 2", versions(app))
	}
}

func TestAValueThatIsNotANumberCannotBeCommitted(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/server/ports/0")
	app.Do(ActionEdit{})

	before := app.doc.Root()

	answer(app, "80x0")

	if app.doc.Root() != before {
		t.Error("a value that is not a JSON number was written to the document")
	}

	if app.Mode() != ModeEdit {
		t.Errorf("mode = %v, want the prompt still open at %v", app.Mode(), ModeEdit)
	}

	if got := app.Prompt().Error; got != "not a JSON number" {
		t.Errorf("error = %q, want the reason the number was refused", got)
	}

	// The text is still in the box, so the answer is corrected rather than
	// typed again.
	answer(app, "9090")

	if app.Prompt().Kind != PromptNone {
		t.Errorf("the corrected answer was not taken: %q", app.Prompt().Error)
	}
}

func TestTypingSaysWhyAnAnswerCannotBeCommitted(t *testing.T) {
	t.Parallel()

	// The refusal appears while the value is being typed rather than when
	// Enter is pressed, and goes away when the text becomes a number again.
	app := session(t, sample(t))
	standOn(t, app, "/server/ports/0")
	app.Do(ActionEdit{})

	app.Do(ActionPromptChange{Text: "80x"})

	if app.Prompt().Error == "" {
		t.Error("a value that could not be committed was reported as fine")
	}

	if app.doc.IsDirty() {
		t.Error("typing changed the document")
	}

	app.Do(ActionPromptChange{Text: "80"})

	if got := app.Prompt().Error; got != "" {
		t.Errorf("error = %q, want none once the text is a number", got)
	}
}

func TestCommittingTheValueAlreadyThereAddsNoVersion(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/server/ports/0")
	app.Do(ActionEdit{})
	answer(app, "8080")

	if versions(app) != 1 {
		t.Errorf("%d versions, want 1: nothing about the document changed", versions(app))
	}

	if app.doc.IsDirty() {
		t.Error("the document is marked unsaved after an edit that changed nothing")
	}
}

func TestRenamingAMemberChangesItsKey(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/name")

	in := beginInput(t, app.Do(ActionRenameKey{}))

	if in.Text != "name" {
		t.Errorf("the text box holds %q, want the key as it stands", in.Text)
	}

	if p := app.Prompt(); p.Title != "New key" || !p.Multiline {
		t.Errorf("prompt = %q multiline=%v, want %q, which is a string like any other", p.Title, p.Multiline, "New key")
	}

	answer(app, "title")

	if _, ok := domain.Resolve(app.doc.Root(), pointer(t, "/name")); ok {
		t.Error("the old key is still in the document")
	}

	if got := nodeAt(t, app, "/title").(*domain.String).Value(); got != "pino" {
		t.Errorf("the renamed member holds %q, want the value it had", got)
	}

	if got := cursorOf(app); got != "/title" {
		t.Errorf("cursor at %q, want the member under its new name", got)
	}
}

func TestRenamingRefusesWhatHasNoKey(t *testing.T) {
	t.Parallel()

	// An element of an array is named by where it sits, and the root is a
	// member of nothing. Neither opens a prompt that could not be answered.
	for _, pointer := range []string{"", "/server/ports/0"} {
		t.Run(pointer, func(t *testing.T) {
			t.Parallel()

			app := session(t, sample(t))
			standOn(t, app, pointer)

			if effects := app.Do(ActionRenameKey{}); len(effects) != 0 {
				t.Errorf("renaming %q asked for a text box", pointer)
			}

			if app.Mode() != ModeNormal || app.Prompt().Kind != PromptNone {
				t.Errorf("renaming %q opened a prompt", pointer)
			}
		})
	}
}

func TestAKeyAnotherMemberHasCannotBeCommitted(t *testing.T) {
	t.Parallel()

	// Two members with one key would make a path name two nodes, which the
	// cursor, the folded set and undo all rest on not happening.
	app := session(t, sample(t))
	standOn(t, app, "/name")
	app.Do(ActionRenameKey{})

	before := app.doc.Root()

	answer(app, "debug")

	if app.doc.Root() != before {
		t.Error("a key another member has was written to the document")
	}

	if app.Mode() != ModeEdit {
		t.Errorf("mode = %v, want the prompt still open at %v", app.Mode(), ModeEdit)
	}

	if got := app.Prompt().Error; got == "" {
		t.Error("the prompt does not say why the key was refused")
	}
}

func TestRenamingToTheKeyAlreadyThereAddsNoVersion(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/name")
	app.Do(ActionRenameKey{})
	answer(app, "name")

	if versions(app) != 1 {
		t.Errorf("%d versions, want 1: nothing about the document changed", versions(app))
	}

	if app.doc.IsDirty() {
		t.Error("the document is marked unsaved after an edit that changed nothing")
	}

	if app.Mode() != ModeNormal {
		t.Error("the prompt stayed open after the answer was taken")
	}
}

func TestChangingTypeOffersTheSixTypes(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/name")
	app.Do(ActionChangeType{})

	p := app.Prompt()
	if p.Kind != PromptChoice {
		t.Fatalf("prompt = %v, want a choice", p.Kind)
	}

	if got := keysOf(p); got != "snbzoa" {
		t.Errorf("keys = %q, want the six types", got)
	}

	want := []string{"string", "number", "boolean", "null", "object", "array"}
	for i, c := range p.Choices {
		if c.Label != want[i] {
			t.Errorf("choice %d is %q, want %q", i, c.Label, want[i])
		}
	}

	if app.Mode() != ModeEdit {
		t.Errorf("mode = %v, want %v", app.Mode(), ModeEdit)
	}
}

func TestChangingTheTypeOfAValueHappensAtOnce(t *testing.T) {
	t.Parallel()

	// Nothing is lost when a value becomes another value, so nothing is asked.
	app := session(t, sample(t))
	standOn(t, app, "/name")
	app.Do(ActionChangeType{})
	pick(app, 'n')

	n, ok := nodeAt(t, app, "/name").(*domain.Number)
	if !ok {
		t.Fatalf("the node is a %v, want a number", nodeAt(t, app, "/name").Kind())
	}

	// "pino" is not a number, so the zero value of the type asked for is what
	// comes back.
	if n.Raw() != "0" {
		t.Errorf("the value is %q, want the zero of the type", n.Raw())
	}

	if app.Mode() != ModeNormal || versions(app) != 2 {
		t.Errorf("mode = %v, %d versions, want %v and 2", app.Mode(), versions(app), ModeNormal)
	}
}

func TestChangingTheTypeOfAFullContainerAsksFirst(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/server")

	before := app.doc.Root()

	app.Do(ActionChangeType{})
	pick(app, 'a')

	p := app.Prompt()
	if p.Kind != PromptChoice || p.Title != "Discard 5 child nodes under /server?" {
		t.Errorf("prompt = %v %q, want the whole subtree counted", p.Kind, p.Title)
	}

	if got := keysOf(p); got != "yn" {
		t.Errorf("keys = %q, want yes and no", got)
	}

	if app.Mode() != ModeConfirm {
		t.Errorf("mode = %v, want %v", app.Mode(), ModeConfirm)
	}

	if app.doc.Root() != before {
		t.Fatal("the children went before the question was answered")
	}

	pick(app, 'y')

	if got := nodeAt(t, app, "/server").Kind(); got != domain.KindArray {
		t.Errorf("the node is a %v, want an array", got)
	}

	if app.Mode() != ModeNormal {
		t.Errorf("mode = %v, want %v once the answer is in", app.Mode(), ModeNormal)
	}
}

func TestTheCountAsksAboutTheWholeSubtree(t *testing.T) {
	t.Parallel()

	// One node under it, and the question says so in the singular. The root is
	// spelled "/" here, where the empty pointer would read as a gap.
	app := session(t, nested(t))

	for pointer, want := range map[string]string{
		"/a": "Discard 1 child node under /a?",
		"":   "Discard 3 child nodes under /?",
	} {
		standOn(t, app, pointer)
		app.Do(ActionChangeType{})
		pick(app, 's')

		if got := app.Prompt().Title; got != want {
			t.Errorf("question for %q is %q, want %q", pointer, got, want)
		}

		app.Do(ActionCancel{})
	}
}

func TestAnEmptyContainerChangesTypeWithoutAsking(t *testing.T) {
	t.Parallel()

	// There is nothing under it to discard, so there is nothing to agree to.
	app := session(t, nested(t))
	standOn(t, app, "/empty")
	app.Do(ActionChangeType{})
	pick(app, 's')

	if app.Mode() != ModeNormal {
		t.Errorf("mode = %v, want %v: an empty container loses nothing", app.Mode(), ModeNormal)
	}

	if got := nodeAt(t, app, "/empty").Kind(); got != domain.KindString {
		t.Errorf("the node is a %v, want a string", got)
	}
}

func TestSayingNoLeavesTheDocumentAlone(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/server")

	before := app.doc.Root()

	app.Do(ActionChangeType{})
	pick(app, 's')
	pick(app, 'n')

	if app.doc.Root() != before {
		t.Error("the type changed after the question was answered with no")
	}

	if app.Mode() != ModeNormal || versions(app) != 1 {
		t.Errorf("mode = %v, %d versions, want %v and 1", app.Mode(), versions(app), ModeNormal)
	}
}

func TestChoosingTheTypeANodeAlreadyHasAddsNoVersion(t *testing.T) {
	t.Parallel()

	// The document does not change, so no version is pushed and no question is
	// asked — even of a container, which would otherwise be losing children.
	for _, pointer := range []string{"/name", "/server"} {
		t.Run(pointer, func(t *testing.T) {
			t.Parallel()

			app := session(t, sample(t))
			standOn(t, app, pointer)
			app.Do(ActionChangeType{})

			if pointer == "/name" {
				pick(app, 's')
			} else {
				pick(app, 'o')
			}

			if versions(app) != 1 || app.doc.IsDirty() {
				t.Errorf("%d versions, dirty=%v, want 1 and false", versions(app), app.doc.IsDirty())
			}

			if app.Mode() != ModeNormal {
				t.Errorf("mode = %v, want %v", app.Mode(), ModeNormal)
			}
		})
	}
}

func TestAddingAnObjectMemberCollectsAKeyTypeAndValue(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))

	in := beginInput(t, app.Do(ActionAddChild{}))
	if in.Text != "" || !in.Multiline {
		t.Errorf("the key box holds %q multiline=%v, want an empty JSON string box", in.Text, in.Multiline)
	}

	if p := app.Prompt(); p.Title != "New key" || app.Mode() != ModeInsert {
		t.Errorf("prompt = %q mode=%v, want %q in %v", p.Title, app.Mode(), "New key", ModeInsert)
	}

	answer(app, "timeout")

	if p := app.Prompt(); p.Kind != PromptChoice || p.Title != "type" {
		t.Fatalf("prompt = %v %q, want the type choices", p.Kind, p.Title)
	}

	in = beginInput(t, app.Do(ActionPromptChoose{Key: 'n'}))
	if in.Text != "0" || in.Multiline {
		t.Errorf("the number box holds %q multiline=%v, want %q on one line", in.Text, in.Multiline, "0")
	}

	answer(app, "30")

	if got := nodeAt(t, app, "/timeout").(*domain.Number).Raw(); got != "30" {
		t.Errorf("the inserted value is %q, want %q", got, "30")
	}

	o := app.doc.Root().(*domain.Object)
	if got := o.At(o.Len() - 1).Key; got != "timeout" {
		t.Errorf("the last key is %q, want the appended member", got)
	}

	if got := cursorOf(app); got != "/timeout" {
		t.Errorf("cursor at %q, want the inserted member", got)
	}

	if app.Mode() != ModeNormal || versions(app) != 2 {
		t.Errorf("mode=%v versions=%d, want %v and 2", app.Mode(), versions(app), ModeNormal)
	}
}

func TestAddingAnArrayElementStartsWithItsType(t *testing.T) {
	t.Parallel()

	app := session(t, domain.NewArray(nil))

	if effects := app.Do(ActionAddChild{}); len(effects) != 0 {
		t.Errorf("adding to an array asked for %d effects before its type was chosen", len(effects))
	}

	if p := app.Prompt(); p.Kind != PromptChoice || p.Title != "type" {
		t.Fatalf("prompt = %v %q, want the type choices", p.Kind, p.Title)
	}

	pick(app, 'b')

	inserted := nodeAt(t, app, "/0")
	b, ok := inserted.(*domain.Bool)
	if !ok {
		t.Fatalf("the inserted value is a %T, want a boolean", inserted)
	}

	if b.Value() {
		t.Error("the inserted boolean is true, want false")
	}

	if got := cursorOf(app); got != "/0" {
		t.Errorf("cursor at %q, want the inserted element", got)
	}
}

func TestAnInsertedNumberThatIsNotJSONCannotBeCommitted(t *testing.T) {
	t.Parallel()

	app := session(t, domain.NewArray(nil))
	before := app.doc.Root()
	app.Do(ActionAddChild{})
	beginInput(t, app.Do(ActionPromptChoose{Key: 'n'}))
	answer(app, "01")

	if app.doc.Root() != before {
		t.Error("the invalid number was inserted")
	}

	if app.Mode() != ModeInsert || app.Prompt().Kind != PromptText {
		t.Errorf("mode=%v prompt=%v, want the number box to stay open", app.Mode(), app.Prompt().Kind)
	}

	if got := app.Prompt().Error; got == "" {
		t.Error("the number box does not say why the value was refused")
	}
}

func TestAddingATypeWithAZeroValueDoesNotAskForAValue(t *testing.T) {
	t.Parallel()

	for key, want := range map[rune]domain.Kind{
		'b': domain.KindBool,
		'z': domain.KindNull,
		'o': domain.KindObject,
		'a': domain.KindArray,
	} {
		t.Run(string(key), func(t *testing.T) {
			t.Parallel()

			app := session(t, domain.NewArray(nil))
			app.Do(ActionAddChild{})

			if effects := app.Do(ActionPromptChoose{Key: key}); len(effects) != 0 {
				t.Errorf("choosing %v asked for a value box", want)
			}

			if got := nodeAt(t, app, "/0").Kind(); got != want {
				t.Errorf("inserted %v, want %v", got, want)
			}

			if app.Mode() != ModeNormal {
				t.Errorf("mode = %v, want %v", app.Mode(), ModeNormal)
			}
		})
	}
}

func TestRevisionLabelsUseOneSpellingForEveryOperation(t *testing.T) {
	t.Parallel()

	p := path(domain.KeySegment("server"), domain.KeySegment("port"))
	tests := map[operation]string{
		opEditValue:  "edit /server/port",
		opEditKey:    "rename /server/port",
		opChangeType: "type /server/port",
		opInsert:     "insert /server/port",
		opDelete:     "delete /server/port",
	}

	for op, want := range tests {
		if got := revisionLabel(op, p); got != want {
			t.Errorf("revisionLabel(%v) = %q, want %q", op, got, want)
		}
	}
}

func TestAddingASiblingPutsItImmediatelyAfterTheSelection(t *testing.T) {
	t.Parallel()

	t.Run("object", func(t *testing.T) {
		t.Parallel()

		app := session(t, sample(t))
		standOn(t, app, "/name")
		app.Do(ActionAddSibling{})
		answer(app, "version")
		pick(app, 'z')

		o := app.doc.Root().(*domain.Object)
		if got := o.At(1).Key; got != "version" {
			t.Errorf("the second key is %q, want the inserted key", got)
		}
	})

	t.Run("array", func(t *testing.T) {
		t.Parallel()

		app := session(t, sample(t))
		standOn(t, app, "/server/ports/0")
		app.Do(ActionAddSibling{})
		beginInput(t, app.Do(ActionPromptChoose{Key: 'n'}))
		answer(app, "9090")

		ports := nodeAt(t, app, "/server/ports").(*domain.Array)
		if got := ports.At(1).(*domain.Number).Raw(); got != "9090" {
			t.Errorf("the second value is %q, want the inserted value", got)
		}

		if got := ports.At(2).(*domain.Number).Raw(); got != "8443" {
			t.Errorf("the old second value moved to %q, want %q", got, "8443")
		}
	})
}

func TestAnEmptyObjectKeyCanBeInserted(t *testing.T) {
	t.Parallel()

	app := session(t, object(t))
	app.Do(ActionAddChild{})
	answer(app, "")
	pick(app, 'z')

	if _, ok := domain.Resolve(app.doc.Root(), path(domain.KeySegment(""))); !ok {
		t.Error("the member with an empty key was not inserted")
	}
}

func TestAnExistingObjectKeyCannotBeInserted(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	before := app.doc.Root()
	app.Do(ActionAddChild{})
	answer(app, "name")

	if app.doc.Root() != before {
		t.Error("the duplicate member was written to the document")
	}

	if app.Mode() != ModeInsert || app.Prompt().Kind != PromptText {
		t.Errorf("mode=%v prompt=%v, want the key box to stay open", app.Mode(), app.Prompt().Kind)
	}

	if got := app.Prompt().Error; got == "" {
		t.Error("the key box does not say why the key was refused")
	}
}

func TestAddingRefusesAChildOfAPrimitiveAndASiblingOfTheRoot(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/name")

	if effects := app.Do(ActionAddChild{}); len(effects) != 0 {
		t.Errorf("adding a child to a primitive asked for %d effects", len(effects))
	}

	standOn(t, app, "")
	if effects := app.Do(ActionAddSibling{}); len(effects) != 0 {
		t.Errorf("adding a sibling to the root asked for %d effects", len(effects))
	}

	if app.Mode() != ModeNormal || versions(app) != 1 {
		t.Errorf("mode=%v versions=%d, want %v and 1", app.Mode(), versions(app), ModeNormal)
	}
}

func TestAddingToAFoldedContainerOpensIt(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/server/ports")
	app.Do(ActionMoveOut{})

	if !app.view.IsCollapsed(app.view.Cursor) {
		t.Fatal("the array is not folded, so this proves nothing")
	}

	app.Do(ActionAddChild{})
	pick(app, 'b')

	if app.view.IsCollapsed(pointer(t, "/server/ports")) {
		t.Error("the insertion left its parent folded")
	}

	if got := cursorOf(app); got != "/server/ports/2" {
		t.Errorf("cursor at %q, want the new element", got)
	}

	if app.Frame().Cursor < 0 {
		t.Error("the inserted element is not on screen")
	}
}

func TestDeletingAValueHappensImmediately(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/name")
	app.Do(ActionDelete{})

	if _, ok := domain.Resolve(app.doc.Root(), pointer(t, "/name")); ok {
		t.Error("the value is still in the document")
	}

	if got := cursorOf(app); got != "/server" {
		t.Errorf("cursor at %q, want the next sibling", got)
	}

	if app.Prompt().Kind != PromptNone || versions(app) != 2 {
		t.Errorf("prompt=%v versions=%d, want none and 2", app.Prompt().Kind, versions(app))
	}
}

func TestDeletingAnEmptyContainerHappensImmediately(t *testing.T) {
	t.Parallel()

	app := session(t, object(t, member("empty", domain.NewArray(nil))))
	standOn(t, app, "/empty")
	app.Do(ActionDelete{})

	if _, ok := domain.Resolve(app.doc.Root(), pointer(t, "/empty")); ok {
		t.Error("the empty container is still in the document")
	}

	if app.Prompt().Kind != PromptNone || cursorOf(app) != "" {
		t.Errorf("prompt=%v cursor=%q, want no question and the parent", app.Prompt().Kind, cursorOf(app))
	}
}

func TestDeletingAFullContainerAsksFirst(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/server")
	before := app.doc.Root()
	app.Do(ActionDelete{})

	if p := app.Prompt(); p.Kind != PromptChoice || p.Title != "Discard 5 child nodes under /server?" {
		t.Fatalf("prompt = %v %q, want the whole subtree counted", p.Kind, p.Title)
	}

	pick(app, 'n')
	if app.doc.Root() != before || app.Mode() != ModeNormal {
		t.Error("saying no changed the document or left the question open")
	}

	app.Do(ActionDelete{})
	pick(app, 'y')

	if _, ok := domain.Resolve(app.doc.Root(), pointer(t, "/server")); ok {
		t.Error("the confirmed subtree is still in the document")
	}

	if got := cursorOf(app); got != "/debug" {
		t.Errorf("cursor at %q, want the next sibling", got)
	}
}

func TestDeletingFallsBackToThePreviousSiblingAndThenTheParent(t *testing.T) {
	t.Parallel()

	app := session(t, domain.NewArray([]domain.Node{domain.NewNull(), domain.NewNull()}))
	standOn(t, app, "/1")
	app.Do(ActionDelete{})

	if got := cursorOf(app); got != "/0" {
		t.Errorf("cursor at %q, want the previous sibling", got)
	}

	app.Do(ActionDelete{})
	if got := cursorOf(app); got != "" {
		t.Errorf("cursor at %q, want the parent", got)
	}
}

func TestTheDocumentRootCannotBeDeleted(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	before := app.doc.Root()
	app.Do(ActionDelete{})

	if app.doc.Root() != before || app.Prompt().Kind != PromptNone || versions(app) != 1 {
		t.Error("deleting the root changed the document or opened a prompt")
	}
}

func TestAKeyThePromptDoesNotOfferDoesNothing(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/name")
	app.Do(ActionChangeType{})
	pick(app, 'q')

	if app.Mode() != ModeEdit || app.Prompt().Kind != PromptChoice {
		t.Error("a key the prompt does not offer closed it")
	}

	if app.doc.IsDirty() {
		t.Error("a key the prompt does not offer changed the document")
	}
}

func TestEscapeAbandonsAnEditAtAnyStep(t *testing.T) {
	t.Parallel()

	// Every step of every flow, including the ones that have already gathered
	// an answer: a flow commits once, at the end, so there is nothing to undo.
	starts := map[string]struct {
		at      string
		actions []Action
	}{
		"typing a value":       {"/name", []Action{ActionEdit{}, ActionPromptChange{Text: "other"}}},
		"typing a key":         {"/name", []Action{ActionRenameKey{}, ActionPromptChange{Text: "other"}}},
		"choosing a type":      {"/name", []Action{ActionChangeType{}}},
		"answering a question": {"/server", []Action{ActionChangeType{}, ActionPromptChoose{Key: 'a'}}},
		"typing an inserted key": {"", []Action{
			ActionAddChild{}, ActionPromptChange{Text: "other"},
		}},
		"choosing an inserted type": {"/server/ports", []Action{ActionAddChild{}}},
		"typing an inserted value": {"/server/ports", []Action{
			ActionAddChild{}, ActionPromptChoose{Key: 's'}, ActionPromptChange{Text: "other"},
		}},
		"confirming a deletion": {"/server", []Action{ActionDelete{}}},
	}

	for name, start := range starts {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := session(t, sample(t))
			standOn(t, app, start.at)

			actions := start.actions
			before := app.doc.Root()
			press(app, actions...)

			if app.Prompt().Kind == PromptNone {
				t.Fatalf("no prompt opened, so there is nothing to abandon")
			}

			app.Do(ActionCancel{})

			if app.doc.Root() != before {
				t.Error("the document changed although the edit was abandoned")
			}

			if app.Mode() != ModeNormal || app.Prompt().Kind != PromptNone {
				t.Errorf("mode = %v with prompt %v, want %v and none", app.Mode(), app.Prompt().Kind, ModeNormal)
			}

			if versions(app) != 1 {
				t.Errorf("%d versions, want 1", versions(app))
			}
		})
	}
}

func TestEveryEditEndsWithTheCursorOnScreen(t *testing.T) {
	t.Parallel()

	// Editing replaces the rows wholesale, and the cursor is held as a path
	// rather than as a row number. What every edit has to leave behind is a
	// path that is drawn somewhere.
	edits := map[string]struct {
		at      []string
		actions []Action
	}{
		"a flipped boolean": {[]string{"/server/tls", "/debug"}, []Action{ActionEdit{}}},
		"a typed value":     {[]string{"/server/ports/1", "/name"}, []Action{ActionEdit{}, ActionPromptSubmit{Text: "9090"}}},
		"a renamed key":     {[]string{"/name", "/server/host"}, []Action{ActionRenameKey{}, ActionPromptSubmit{Text: "other"}}},
		"a changed type":    {[]string{"/server/tls", "/name", "/server/ports/1"}, []Action{ActionChangeType{}, ActionPromptChoose{Key: 'b'}}},
		"an inserted value": {[]string{"/server/ports"}, []Action{ActionAddChild{}, ActionPromptChoose{Key: 'b'}}},
		"a deleted value":   {[]string{"/server/ports/1", "/name"}, []Action{ActionDelete{}}},
	}

	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for _, pointer := range edit.at {
				app := session(t, sample(t))
				app.Do(ActionResize{Height: 12})
				standOn(t, app, pointer)
				press(app, edit.actions...)

				if got := app.Frame().Cursor; got < 0 {
					t.Errorf("%s at %q left the cursor off the rows", name, pointer)
				}

				if app.Mode() != ModeNormal {
					t.Errorf("%s at %q left the session in %v", name, pointer, app.Mode())
				}
			}
		})
	}
}

func TestEditingFollowsTheFoldedSet(t *testing.T) {
	t.Parallel()

	// Renaming moves a subtree, and what is folded inside it moves with it.
	app := session(t, sample(t))
	standOn(t, app, "/server/ports")
	app.Do(ActionMoveOut{})

	standOn(t, app, "/server")
	app.Do(ActionRenameKey{})
	answer(app, "srv")

	if got := foldsOf(app.view); len(got) != 1 || got[0] != "/srv/ports" {
		t.Errorf("folded %v, want the fold carried to the new name", got)
	}
}

func TestAnEditedNodeInsideAFoldIsShownWhereItIs(t *testing.T) {
	t.Parallel()

	// The edit says where it happened; whether that place is drawn is settled
	// afterwards, by walking out to the nearest row there is.
	app := session(t, sample(t))
	standOn(t, app, "/server")
	app.Do(ActionEdit{}) // fold it away

	standOn(t, app, "/server")
	app.Do(ActionChangeType{})
	pick(app, 'a')
	pick(app, 'y')

	if got := app.Frame().Cursor; got < 0 {
		t.Error("the cursor is on no row after editing inside a fold")
	}
}

func TestTheSameEditsRunTheSameWayInBothViews(t *testing.T) {
	t.Parallel()

	// One tree and two renderers, so an edit cannot come out differently. The
	// trees are built separately in each session, so what is compared between
	// them is what they say rather than which objects they are.
	steps := []struct {
		name    string
		at      string
		actions []Action
	}{
		{"flip a boolean", "/debug", []Action{ActionEdit{}}},
		{"type a value", "/server/ports/0", []Action{ActionEdit{}, ActionPromptSubmit{Text: "9090"}}},
		{"rename a key", "/name", []Action{ActionRenameKey{}, ActionPromptSubmit{Text: "title"}}},
		{"add an array element", "/server/ports", []Action{ActionAddChild{}, ActionPromptChoose{Key: 'b'}}},
		{"delete an array element", "/server/ports/0", []Action{ActionDelete{}}},
		{"fold a container", "/server", []Action{ActionEdit{}}},
		{"change a type", "/server", []Action{ActionChangeType{}, ActionPromptChoose{Key: 'a'}, ActionPromptChoose{Key: 'y'}}},
	}

	json, tree := sessionIn(t, sample(t), ViewJSON), sessionIn(t, sample(t), ViewTree)

	for _, step := range steps {
		standOn(t, json, step.at)
		standOn(t, tree, step.at)

		press(json, step.actions...)
		press(tree, step.actions...)

		if a, b := contentOf(json), contentOf(tree); a != b {
			t.Fatalf("%s left different documents:\nJSON view:\n%s\ntree view:\n%s", step.name, a, b)
		}

		if a, b := cursorOf(json), cursorOf(tree); a != b {
			t.Errorf("%s left the cursor at %q in the JSON view and %q in the tree", step.name, a, b)
		}

		if a, b := foldsOf(json.view), foldsOf(tree.view); !slices.Equal(a, b) {
			t.Errorf("%s left %v folded in the JSON view and %v in the tree", step.name, a, b)
		}
	}
}

func TestASessionWithNothingOpenTakesEditingKeys(t *testing.T) {
	t.Parallel()

	// There is no document to edit and no prompt to answer, and pressing the
	// keys anyway is not a way to reach a nil one.
	app := New(Deps{JSONView: documentview.NewJSONRenderer(), TreeView: documentview.NewTreeRenderer()})

	press(app,
		ActionEdit{}, ActionRenameKey{}, ActionAddChild{}, ActionAddSibling{}, ActionDelete{}, ActionChangeType{},
		ActionPromptChange{Text: "x"}, ActionPromptSubmit{Text: "x"},
		ActionPromptChoose{Key: 's'}, ActionCancel{},
	)

	if app.Mode() != ModeNormal || app.Prompt().Kind != PromptNone {
		t.Errorf("mode = %v with prompt %v, want %v and none", app.Mode(), app.Prompt().Kind, ModeNormal)
	}
}

func TestTypingOverAValueAndCommittingItLeavesTheDocumentAlone(t *testing.T) {
	t.Parallel()

	// The value is handed to whoever draws, typed over by nobody, and handed
	// back. A document changed by having been looked at is the worst thing an
	// editor can do, and the tree says whether it happened: an edit that
	// changes nothing comes back as the very root it was given.
	for name, value := range awkwardValues() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := session(t, object(t, member("v", text(t, value))))
			standOn(t, app, "/v")

			before := app.doc.Root()
			in := beginInput(t, app.Do(ActionEdit{}))

			answer(app, in.Text)

			if app.doc.Root() != before {
				t.Errorf("the document changed: %q became %q",
					value, nodeAt(t, app, "/v").(*domain.String).Value())
			}

			if versions(app) != 1 || app.doc.IsDirty() {
				t.Errorf("%d versions, dirty=%v, want 1 and false", versions(app), app.doc.IsDirty())
			}

			if app.Mode() != ModeNormal {
				t.Errorf("mode = %v, want %v", app.Mode(), ModeNormal)
			}
		})
	}
}

func TestRenamingToTheKeyAlreadyThereLeavesTheDocumentAlone(t *testing.T) {
	t.Parallel()

	// The same property for keys, which are strings and are spelled the same
	// way. A key holding a tab is unusual and legal.
	for name, key := range awkwardValues() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := session(t, object(t, member(key, text(t, "v"))))
			standOn(t, app, domain.Path{}.Child(domain.KeySegment(key)).String())

			before := app.doc.Root()
			in := beginInput(t, app.Do(ActionRenameKey{}))

			answer(app, in.Text)

			if app.doc.Root() != before {
				t.Errorf("the document changed although the key %q was left alone", key)
			}

			if versions(app) != 1 || app.doc.IsDirty() {
				t.Errorf("%d versions, dirty=%v, want 1 and false", versions(app), app.doc.IsDirty())
			}
		})
	}
}

func TestAValueIsOfferedAsTheDocumentSpellsIt(t *testing.T) {
	t.Parallel()

	// What is on screen is what would be saved, and the box is on screen: a
	// row shows "a\tb", so that is what is typed over.
	app := session(t, object(t, member("v", text(t, "a\tb"))))
	standOn(t, app, "/v")

	if got := beginInput(t, app.Do(ActionEdit{})).Text; got != `a\tb` {
		t.Errorf("the box holds %q, want the value as the document spells it", got)
	}
}

func TestAnEscapeThatWasTypedBecomesTheCharacterItNames(t *testing.T) {
	t.Parallel()

	app := session(t, object(t, member("v", text(t, "ab"))))
	standOn(t, app, "/v")
	app.Do(ActionEdit{})
	answer(app, `a\tb`)

	if got := nodeAt(t, app, "/v").(*domain.String).Value(); got != "a\tb" {
		t.Errorf("the value is %q, want a tab between the letters", got)
	}
}

func TestASpellingThatCannotBeReadBackIsRefused(t *testing.T) {
	t.Parallel()

	for name, typed := range map[string]string{
		"an escape nothing means": `a\qb`,
		"a backslash at the end":  `ab\`,
		"half a character":        `a\ud83cb`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := session(t, object(t, member("v", text(t, "ab"))))
			standOn(t, app, "/v")
			app.Do(ActionEdit{})

			before := app.doc.Root()

			answer(app, typed)

			if app.doc.Root() != before {
				t.Error("a spelling that cannot be read back was written to the document")
			}

			if app.Mode() != ModeEdit {
				t.Errorf("mode = %v, want the prompt still open at %v", app.Mode(), ModeEdit)
			}

			if app.Prompt().Error == "" {
				t.Error("the prompt does not say why the answer was refused")
			}
		})
	}
}
