package application

import (
	"errors"
	"strconv"

	"github.com/ytakahashi/pino/internal/domain"
)

// operation is what an edit in progress was asked to do.
type operation uint8

const (
	opEditValue  operation = iota // Enter on a string or a number
	opEditKey                     // r
	opChangeType                  // t, and Enter on a null
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

// flow is an edit in progress: what was asked for, how far it has got, and
// what has been gathered on the way. It is nil in normal mode.
//
// One struct covers every operation rather than one type each behind an
// interface. What differs between them is what to do once the answers are in;
// the steps themselves are of three kinds and no more, so a single struct
// keeps the whole of "where has this got to" readable in one place, at the
// cost of fields that only some operations fill in.
type flow struct {
	op   operation
	step step

	// target is the node being edited.
	target domain.Path

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
func (f *flow) mode() Mode {
	if f.step == stepConfirm {
		return ModeConfirm
	}

	return ModeEdit
}

// title is what a text prompt asks for.
func (f *flow) title() string {
	if f.op == opEditKey {
		return "New key"
	}

	return "Edit " + f.kind.String()
}

// label names the version this flow will produce, as "edit /server/port".
func (f *flow) label() string { return f.verb() + " " + pointerText(f.target) }

func (f *flow) verb() string {
	switch f.op {
	case opEditValue:
		return "edit"

	case opEditKey:
		return "rename"

	case opChangeType:
		return "type"
	}

	// Not reached: the switch covers every operation, and the linter keeps it
	// so.
	return "edit"
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

	return a.beginText(opEditKey, domain.KindString, p.At(p.Len()-1).Token())
}

// changeType is t: ask which type the selected node should become.
func (a *App) changeType() {
	if _, ok := a.selected(); !ok {
		return
	}

	a.beginType()
}

// cancel drops the edit in progress, however far it had got.
//
// The document needs nothing done to it: a flow gathers answers and commits
// once, at the end, so abandoning one is forgetting the answers. That is why
// Esc is the same key at every step.
func (a *App) cancel() { a.flow = nil }

// beginText opens a prompt to type an answer into, seeded with text.
//
// The widget is asked for the shape the prompt says it has rather than for one
// worked out a second time here: whether newlines are allowed is one fact
// about the answer, and the drawing side reads it from PromptInfo on every
// redraw afterwards.
func (a *App) beginText(op operation, kind domain.Kind, text string) []Effect {
	a.flow = &flow{op: op, step: stepText, target: a.view.Cursor, kind: kind}

	return []Effect{EffectBeginInput{Text: text, Multiline: a.Prompt().Multiline}}
}

// beginType opens the list of types. It is the same prompt whether t asked for
// it or Enter on a null did.
func (a *App) beginType() {
	a.flow = &flow{op: opChangeType, step: stepType, target: a.view.Cursor}
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
	if a.flow == nil || a.flow.step != stepText {
		return
	}

	if _, err := a.applyText(text); err != nil {
		a.flow.err = promptError(err)

		return
	}

	a.flow.err = ""
}

// submit is Enter on a text prompt: take the answer, or say why it cannot be
// taken and stay open.
//
// Staying open is what makes a refusal recoverable: the text is still in the
// widget, and a key that could not be committed is corrected rather than typed
// again from the start.
func (a *App) submit(text string) {
	if a.flow == nil || a.flow.step != stepText {
		return
	}

	res, err := a.applyText(text)
	if err != nil {
		a.flow.err = promptError(err)

		return
	}

	a.commit(res, a.flow.label())
	a.flow = nil
}

// choose is a key pressed on a prompt that offers keys.
func (a *App) choose(key rune) {
	if a.flow == nil {
		return
	}

	switch a.flow.step {
	case stepType:
		a.chooseType(key)

	case stepConfirm:
		a.answerConfirm(key)

	case stepText:
		// A text prompt is not answered a key at a time: what has been typed
		// arrives whole, as text.
	}
}

// applyText is the edit the answer to a text prompt asks for, tried against
// the document as it stands.
//
// Two operations ask for text, and everything else about them is the same, so
// the check that separates them is which of the two this is.
func (a *App) applyText(text string) (domain.EditResult, error) {
	if a.flow.op == opEditKey {
		return domain.Rename(a.doc.Root(), a.flow.target, text)
	}

	v, err := scalarFrom(text, a.flow.kind)
	if err != nil {
		return domain.EditResult{}, err
	}

	return domain.SetValue(a.doc.Root(), a.flow.target, v)
}

// scalarFrom is text read as a value of kind k.
//
// A string is taken as it stands; a number has to be one. What a JSON number
// may look like is knowledge about the format, so it is asked of the domain
// rather than checked in the layer that happened to read the keystrokes.
func scalarFrom(text string, k domain.Kind) (domain.Node, error) {
	if k == domain.KindNumber {
		n, err := domain.ParseNumber(text)
		if err != nil {
			return nil, err
		}

		return n, nil
	}

	s, err := domain.NewString(text)
	if err != nil {
		return nil, err
	}

	return s, nil
}

// chooseType takes the type asked for, and either changes to it or asks first.
func (a *App) chooseType(key rune) {
	kind, ok := kindFor(key)
	if !ok {
		return
	}

	n, ok := domain.Resolve(a.doc.Root(), a.flow.target)
	if !ok {
		// The node went away while the prompt was open, which undo can bring
		// about. There is nothing left to change, so the question goes.
		a.flow = nil

		return
	}

	// Nodes are lost when a container becomes something else, and only then:
	// choosing the type a node already has changes nothing at all, and a
	// container with nothing in it has nothing to lose.
	if n.Kind() != kind && domain.CountDescendants(n) > 0 {
		a.flow.kind, a.flow.step = kind, stepConfirm

		return
	}

	a.changeTypeTo(kind)
}

// answerConfirm takes yes or no; any other key is neither and is ignored.
//
// Changing a type is the only thing that asks for confirmation today. Deleting
// will be the other, and will be told apart by the operation.
func (a *App) answerConfirm(key rune) {
	switch key {
	case confirmYes:
		a.changeTypeTo(a.flow.kind)

	case confirmNo:
		a.flow = nil
	}
}

// changeTypeTo carries the change out and closes the flow.
func (a *App) changeTypeTo(k domain.Kind) {
	res, err := domain.ChangeType(a.doc.Root(), a.flow.target, k)
	if err != nil {
		a.flow.err = promptError(err)

		return
	}

	a.commit(res, a.flow.label())
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

	a.commit(res, "edit "+pointerText(a.view.Cursor))
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

		case lines[row].Kind == LineOpen && !lines[row].Path.IsRoot():
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
