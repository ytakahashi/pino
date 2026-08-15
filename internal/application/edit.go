package application

import (
	"errors"
	"strconv"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// operation is what an edit in progress was asked to do.
type operation uint8

const (
	opEditValue  operation = iota // Enter on a string or a number
	opEditKey                     // r
	opChangeType                  // t, and Enter on a null
	opInsert                      // a and A
	opDelete                      // d
)

// step is how far a flow has got, in terms of what is on screen: text to type,
// a list of types to pick from, or a question to answer.
//
// It is held rather than worked out from the operation, because neither says
// where a flow is on its own. Changing a type takes two steps when children
// are about to be lost and one when they are not, and a text step is a value
// in one operation and a key in another.
type step uint8

const (
	stepText step = iota
	stepType
	stepConfirm
)

// editFlow is an edit in progress: what was asked for, how far it has got,
// and what has been gathered on the way.
//
// One struct covers every editing operation rather than one type each. What
// differs between them is what to do once the answers are in; the steps
// themselves are of three kinds and no more, so a single struct keeps the
// whole of "where has this got to" readable in one place, at the cost of
// fields that only some operations fill in.
type editFlow struct {
	op   operation
	step step

	// target is the node being edited or deleted. Insertions use parent and at
	// instead because the node they will create has no path yet.
	target domain.Path

	// parent and at say where an insertion will be made. They are fixed when
	// the flow starts, before any answers are collected, so switching views or
	// redrawing cannot change the requested position.
	parent domain.Path
	at     int

	// key is the object key collected before an inserted value's type. Empty
	// is a valid JSON object key, so keySet distinguishes it from no answer yet.
	// Array insertions set keySet from the start and keep key empty.
	key    string
	keySet bool

	// kind is the type the answer is read as: the kind of the value being
	// typed over, or the kind chosen from the list of types and waiting to be
	// confirmed.
	kind domain.Kind

	// err is why the last answer was refused. It is the only thing this side
	// remembers about what has been typed: the text itself is held by the
	// widget collecting it, and a second copy here is a copy that could
	// disagree with what is on screen.
	err string
}

// mode is which of the modes this flow puts the session in.
//
// Deriving it is what makes "a confirmation with nothing to confirm" a state
// nobody can write. Held as a field beside the flow, the two could disagree,
// and the screen would take keys that led nowhere.
func (f *editFlow) mode() Mode {
	if f.step == stepConfirm {
		return ModeConfirm
	}
	if f.op == opInsert {
		return ModeInsert
	}

	return ModeEdit
}

// title is what a text prompt asks for.
func (f *editFlow) title() string {
	if f.op == opEditKey || (f.op == opInsert && !f.keySet) {
		return "New key"
	}

	return "Edit " + f.kind.String()
}

// revisionLabel names a version by the operation and the place it changed.
// Keeping the spelling here prevents one edit from drifting away from the
// labels every other edit writes to history.
func revisionLabel(op operation, p domain.Path) string {
	var verb string

	switch op {
	case opEditValue:
		verb = "edit"

	case opEditKey:
		verb = "rename"

	case opChangeType:
		verb = "type"

	case opInsert:
		verb = "insert"

	case opDelete:
		verb = "delete"
	}

	return verb + " " + pointerText(p)
}

// editing is the edit in progress, and false when the session is in another
// flow or in none.
//
// The answers to a prompt are taken through here rather than from the field
// directly, so that a text answer arriving while a quit is being confirmed is
// answered by doing nothing. The terminal cannot send one — the prompt on
// screen is the one the flow described — but an Action driven straight at this
// layer must not reach into a flow that is not there.
func (a *App) editing() (*editFlow, bool) {
	f, ok := a.flow.(*editFlow)

	return f, ok
}

// selected is the node the cursor is on.
//
// It reports false when no document is open as well as when the cursor names
// nothing in the one that is. The two are the ways an edit can arrive with
// nothing to edit, and both are answered by doing nothing, so both are asked
// for here rather than at each of the keys.
func (a *App) selected() (domain.Node, bool) {
	if a.doc == nil {
		return nil, false
	}

	return domain.Resolve(a.doc.Root(), a.view.Cursor)
}

// edit is Enter: act on the selected node, whatever acting on it means.
//
// The six kinds fall into four answers. A string or a number is typed over. A
// boolean has two states and is flipped outright, since a prompt offering a
// choice of two would spend three keystrokes to save none. A container has no
// value to edit — a and d and t are what change its contents — so it folds or
// unfolds, which is what pressing Enter on a row holding other rows most
// plainly means. A null has no value either, so it asks which type to become,
// which is the question t asks.
func (a *App) edit() []Effect {
	n, ok := a.selected()
	if !ok {
		return nil
	}

	switch n.Kind() {
	case domain.KindString:
		return a.beginText(opEditValue, domain.KindString, n.(*domain.String).Value())

	case domain.KindNumber:
		// The literal as the document spells it, so that editing 1.50 starts
		// from 1.50 rather than from a number reformatted on the way in.
		return a.beginText(opEditValue, domain.KindNumber, n.(*domain.Number).Raw())

	case domain.KindBool:
		a.toggleBool(n.(*domain.Bool))

	case domain.KindNull:
		a.beginType()

	case domain.KindObject, domain.KindArray:
		a.toggleFold()
	}

	return nil
}

// renameKey is r: change the key of the selected member.
//
// Only a member of an object has a key to change. An element of an array is
// named by where it sits, which is not a name anyone chose, and the root is a
// member of nothing. Both are refused here rather than at the end, so that no
// prompt opens on a question that could not be answered.
func (a *App) renameKey() []Effect {
	if _, ok := a.selected(); !ok {
		return nil
	}

	p := a.view.Cursor
	if p.IsRoot() {
		return nil
	}

	parent, ok := domain.Resolve(a.doc.Root(), p.Parent())
	if !ok || parent.Kind() != domain.KindObject {
		return nil
	}

	// Spelled as the value of a string is, since a key is one.
	return a.beginText(opEditKey, domain.KindString, p.At(p.Len()-1).Token())
}

// changeType is t: ask which type the selected node should become.
func (a *App) changeType() {
	if _, ok := a.selected(); !ok {
		return
	}

	a.beginType()
}

// addChild is a: append to the selected container.
func (a *App) addChild() []Effect {
	n, ok := a.selected()
	if !ok {
		return nil
	}

	switch n.Kind() {
	case domain.KindObject:
		return a.beginInsert(a.view.Cursor, n.(*domain.Object).Len(), domain.KindObject)

	case domain.KindArray:
		return a.beginInsert(a.view.Cursor, n.(*domain.Array).Len(), domain.KindArray)

	case domain.KindString, domain.KindNumber, domain.KindBool, domain.KindNull:
		return nil
	}

	return nil
}

// addSibling is A: insert immediately after the selected node.
func (a *App) addSibling() []Effect {
	if _, ok := a.selected(); !ok || a.view.Cursor.IsRoot() {
		return nil
	}

	parentPath := a.view.Cursor.Parent()
	parent, ok := domain.Resolve(a.doc.Root(), parentPath)
	if !ok {
		return nil
	}

	target := a.view.Cursor
	at, ok := domain.ChildIndex(parent, target.At(target.Len()-1))
	if !ok {
		return nil
	}

	return a.beginInsert(parentPath, at+1, parent.Kind())
}

// beginInsert fixes an insertion point and asks the first question needed by
// its container. Objects need a key before a type; arrays start with the type.
func (a *App) beginInsert(parent domain.Path, at int, parentKind domain.Kind) []Effect {
	f := &editFlow{op: opInsert, parent: parent, at: at}
	a.flow = f

	if parentKind == domain.KindObject {
		f.step, f.kind = stepText, domain.KindString

		return a.inputEffect(f, "")
	}

	f.step, f.keySet = stepType, true

	return nil
}

// deleteSelected is d: delete at once when there are no descendants, and ask
// before throwing away a populated subtree.
func (a *App) deleteSelected() {
	n, ok := a.selected()
	if !ok || a.view.Cursor.IsRoot() {
		return
	}

	target := a.view.Cursor
	if domain.CountDescendants(n) > 0 {
		a.flow = &editFlow{op: opDelete, step: stepConfirm, target: target}

		return
	}

	a.deleteAt(target)
}

// cancel drops whatever is in progress, however far it had got.
//
// The document needs nothing done to it: a flow gathers answers and commits
// once, at the end, so abandoning one is forgetting the answers. That is why
// Esc is the same key at every step of every flow.
func (a *App) cancel() { a.flow = nil }

// beginText opens a prompt to type an answer into, seeded with value.
//
// A string is seeded in the spelling the document uses rather than in the
// characters it holds: a terminal cannot show a tab apart from four spaces or
// hold a control character at all, so a value typed over in that form would
// come back changed by having been looked at. A number has no spelling to
// undo and is seeded as it stands.
//
// The widget is asked for the shape the prompt says it has rather than for one
// worked out a second time here: whether newlines are allowed is one fact
// about the answer, and the drawing side reads it from PromptInfo on every
// redraw afterwards.
func (a *App) beginText(op operation, kind domain.Kind, value string) []Effect {
	f := &editFlow{op: op, step: stepText, target: a.view.Cursor, kind: kind}
	a.flow = f

	return a.inputEffect(f, value)
}

// inputEffect asks the terminal for a box matching f's current text step.
func (a *App) inputEffect(f *editFlow, value string) []Effect {
	kind := f.kind

	text, oneLine := value, value
	if kind == domain.KindString {
		text, oneLine = domain.EditableText(value), domain.EditableLine(value)
	}

	return []Effect{EffectBeginInput{
		Text:      text,
		OneLine:   oneLine,
		Multiline: a.Prompt().Multiline,
	}}
}

// beginType opens the list of types. It is the same prompt whether t asked for
// it or Enter on a null did.
func (a *App) beginType() {
	a.flow = &editFlow{op: opChangeType, step: stepType, target: a.view.Cursor}
}

// validate says whether what has been typed so far could be committed, so that
// a refusal is on screen while it is being typed rather than after Enter.
//
// It only ever writes the reason down. Checking an answer and carrying it out
// are the same work here, done twice: the edits are pure functions of the
// tree, so trying one and throwing the result away costs the depth of the
// document and needs no second implementation of the rules — and a second one
// is exactly what could say "ok" to an edit the tree would refuse.
func (a *App) validate(text string) {
	f, ok := a.editing()
	if !ok || f.step != stepText {
		return
	}

	var err error
	if f.op == opInsert && !f.keySet {
		_, err = a.validateInsertKey(f, text)
	} else {
		_, err = a.applyText(f, text)
	}

	if err != nil {
		f.err = promptError(err)

		return
	}

	f.err = ""
}

// submit is Enter on a text prompt: take the answer, or say why it cannot be
// taken and stay open.
//
// Staying open is what makes a refusal recoverable: the text is still in the
// widget, and a key that could not be committed is corrected rather than typed
// again from the start.
func (a *App) submit(text string) {
	f, ok := a.editing()
	if !ok || f.step != stepText {
		return
	}

	if f.op == opInsert && !f.keySet {
		key, err := a.validateInsertKey(f, text)
		if err != nil {
			f.err = promptError(err)

			return
		}

		f.key, f.keySet = key, true
		f.step, f.err = stepType, ""

		return
	}

	res, err := a.applyText(f, text)
	if err != nil {
		f.err = promptError(err)

		return
	}

	if f.op == opInsert {
		a.finishInsert(f, res)

		return
	}

	a.commit(res, revisionLabel(f.op, f.target))
	a.flow = nil
}

// choose is a key pressed on a prompt that offers keys.
func (a *App) choose(key rune) []Effect {
	f, ok := a.editing()
	if !ok {
		return nil
	}

	switch f.step {
	case stepType:
		return a.chooseType(f, key)

	case stepConfirm:
		a.answerConfirm(f, key)

	case stepText:
		// A text prompt is not answered a key at a time: what has been typed
		// arrives whole, as text.
	}

	return nil
}

// applyText is the edit the answer to a text prompt asks for, tried against
// the document as it stands.
//
// Two operations ask for text, and everything else about them is the same, so
// the check that separates them is which of the two this is.
func (a *App) applyText(f *editFlow, text string) (domain.EditResult, error) {
	if f.op == opEditKey {
		key, err := domain.ParseEditableText(text)
		if err != nil {
			return domain.EditResult{}, err
		}

		return domain.Rename(a.doc.Root(), f.target, key)
	}

	v, err := scalarFrom(text, f.kind)
	if err != nil {
		return domain.EditResult{}, err
	}

	if f.op == opInsert {
		return a.insertValue(f, f.key, v)
	}

	return domain.SetValue(a.doc.Root(), f.target, v)
}

// validateInsertKey checks a proposed key by attempting the real insertion
// with a disposable null value. The resulting tree is discarded; using the
// domain operation keeps duplicate-key and UTF-8 validation identical to the
// final insertion rather than copying either rule into this layer.
func (a *App) validateInsertKey(f *editFlow, text string) (string, error) {
	key, err := domain.ParseEditableText(text)
	if err != nil {
		return "", err
	}

	_, err = a.insertValue(f, key, domain.NewNull())

	return key, err
}

// insertValue is f's insertion carrying key and value.
func (a *App) insertValue(f *editFlow, key string, value domain.Node) (domain.EditResult, error) {
	return domain.Insert(a.doc.Root(), f.parent, f.at, domain.Member{
		Key:   key,
		Value: value,
	})
}

// scalarFrom is text read as a value of kind k.
//
// A string is read back out of the spelling it was typed in; a number has no
// spelling to undo and has to be a number. Both rules are the format's, so
// both are asked of the domain rather than checked in the layer that happened
// to read the keystrokes.
func scalarFrom(text string, k domain.Kind) (domain.Node, error) {
	if k == domain.KindNumber {
		n, err := domain.ParseNumber(text)
		if err != nil {
			return nil, err
		}

		return n, nil
	}

	value, err := domain.ParseEditableText(text)
	if err != nil {
		return nil, err
	}

	s, err := domain.NewString(value)
	if err != nil {
		return nil, err
	}

	return s, nil
}

// chooseType takes the type asked for, and either changes to it or asks first.
func (a *App) chooseType(f *editFlow, key rune) []Effect {
	kind, ok := kindFor(key)
	if !ok {
		return nil
	}

	if f.op == opInsert {
		return a.chooseInsertType(f, kind)
	}

	n, ok := domain.Resolve(a.doc.Root(), f.target)
	if !ok {
		// A version change abandons its flow, so no current action can reach
		// here. Check anyway rather than relying only on the invariant that a
		// flow and its root are replaced together.
		a.flow = nil

		return nil
	}

	// Nodes are lost when a container becomes something else, and only then:
	// choosing the type a node already has changes nothing at all, and a
	// container with nothing in it has nothing to lose.
	if n.Kind() != kind && domain.CountDescendants(n) > 0 {
		f.kind, f.step = kind, stepConfirm

		return nil
	}

	a.changeTypeTo(f, kind)

	return nil
}

// chooseInsertType either opens the value box for a scalar with a spelling,
// or inserts the chosen type's zero value immediately.
func (a *App) chooseInsertType(f *editFlow, kind domain.Kind) []Effect {
	f.kind = kind

	switch kind {
	case domain.KindString:
		f.step = stepText

		return a.inputEffect(f, "")

	case domain.KindNumber:
		f.step = stepText

		return a.inputEffect(f, "0")

	case domain.KindBool, domain.KindNull, domain.KindObject, domain.KindArray:
		value, err := domain.Convert(domain.NewNull(), kind)
		if err != nil {
			f.err = promptError(err)

			return nil
		}

		res, err := a.insertValue(f, f.key, value)
		if err != nil {
			f.err = promptError(err)

			return nil
		}

		a.finishInsert(f, res)
	}

	return nil
}

// finishInsert opens the destination before commit settles the new cursor.
// Settling first would move a cursor inside a folded parent back onto that
// parent, losing the one place the insertion result says to stand.
func (a *App) finishInsert(f *editFlow, res domain.EditResult) {
	a.view.Expand(f.parent)
	a.commit(res, revisionLabel(opInsert, res.Cursor))
	a.flow = nil
}

// answerConfirm takes yes or no; any other key is neither and is ignored.
//
// Changing a type and deleting are told apart by the operation gathered when
// the question was opened.
func (a *App) answerConfirm(f *editFlow, key rune) {
	switch key {
	case confirmYes:
		switch f.op {
		case opChangeType:
			a.changeTypeTo(f, f.kind)

		case opDelete:
			a.deleteAt(f.target)

		case opEditValue, opEditKey, opInsert:
			// These operations never ask a confirmation question.
		}

	case confirmNo:
		a.flow = nil
	}
}

// deleteAt removes target and closes any confirmation flow around it.
func (a *App) deleteAt(target domain.Path) {
	res, err := domain.Delete(a.doc.Root(), target)
	if err != nil {
		// A flow is tied to the root it gathered its answers from. If its target
		// no longer exists, there is no useful question to leave open.
		a.flow = nil

		return
	}

	a.commit(res, revisionLabel(opDelete, target))
	a.flow = nil
}

// changeTypeTo carries the change out and closes the flow.
func (a *App) changeTypeTo(f *editFlow, k domain.Kind) {
	res, err := domain.ChangeType(a.doc.Root(), f.target, k)
	if err != nil {
		f.err = promptError(err)

		return
	}

	a.commit(res, revisionLabel(f.op, f.target))
	a.flow = nil
}

// toggleBool flips the selected boolean and makes that a version of its own.
//
// It is the whole of an edit with no prompt in front of it: the document is
// changed, the version is pushed, and the flow that was never started is not
// ended.
func (a *App) toggleBool(b *domain.Bool) {
	res, err := domain.SetValue(a.doc.Root(), a.view.Cursor, domain.NewBool(!b.Value()))
	if err != nil {
		// Not reached: the path resolved to the very node being replaced a
		// moment ago. Doing nothing is what a refused edit gets everywhere
		// else here.
		return
	}

	a.commit(res, revisionLabel(opEditValue, a.view.Cursor))
}

// toggleFold folds the selected container away, or unfolds it.
//
// Which of the two it is is asked of the rows rather than of the folded set,
// so that the rule is the one h and l already follow. The root is drawn open
// and has no folded form, and an empty container is drawn as neither open nor
// folded, so both are left alone without a case of their own.
func (a *App) toggleFold() {
	lines := a.render()

	if row := visibleRow(lines, a.view.Cursor); row >= 0 {
		switch {
		case lines[row].Collapsed:
			if a.view.Expand(lines[row].Path) {
				// What was hidden is back, so the rows in hand are stale.
				a.settle(a.render())

				return
			}

		case lines[row].Kind == documentview.LineOpen && !lines[row].Path.IsRoot():
			if a.view.Collapse(lines[row].Path) {
				a.settle(a.render())

				return
			}
		}
	}

	a.settle(lines)
}

// commit makes what an edit produced the current version of the document.
//
// Every edit comes through here, which is what makes the history, the folded
// set, the cursor and the window agree after all of them rather than after
// each having remembered to arrange it.
//
// An edit that changed nothing is not one. The domain hands back the very root
// it was given when the new value is the one already there, and that single
// fact stands for three: no empty version on the stack, no unsaved mark, and
// no folded set walked over paths that never moved.
func (a *App) commit(r domain.EditResult, label string) {
	if r.Root == a.doc.Root() {
		return
	}

	a.doc.Replace(r.Root)
	a.history.Push(Revision{Root: r.Root, Cursor: r.Cursor, Label: label})

	// The edit says where it happened and which paths moved; whether the place
	// it names is on screen is settled afterwards.
	a.view.Apply(r)
	a.settle(a.render())
}

// promptError is why an answer was refused, in the words the person who typed
// it needs.
//
// The domain says what is wrong in terms of the document. Most of what it says
// is already a phrase written to be read, and those are passed through rather
// than restated, so that a rule and its explanation stay in the same place.
func promptError(err error) string {
	var num *domain.InvalidNumberError
	if errors.As(err, &num) {
		return num.Reason
	}

	var escape *domain.InvalidEscapeError
	if errors.As(err, &escape) {
		return escape.Reason
	}

	var dup *domain.DuplicateKeyError
	if errors.As(err, &dup) {
		return "a member named " + strconv.Quote(dup.Key) + " is already here"
	}

	var refused *domain.EditError
	if errors.As(err, &refused) {
		return refused.Reason
	}

	return err.Error()
}
