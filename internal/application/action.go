package application

// Action is a request from the user, already stripped of how it was made.
//
// The presentation layer maps a key press to an Action and knows nothing of
// what it will do; this layer decides that. The pair splits the interaction
// in two along a line that can be tested from both sides: a key table on one
// side, a state transition on the other.
//
// Actions carry their arguments, so an Action is a value rather than an enum:
// editing a value has text to pass along, moving does not.
type Action interface{ isAction() }

// ActionQuit asks to leave pino.
type ActionQuit struct{}

func (ActionQuit) isAction() {}

// ActionMoveNext and ActionMovePrev ask for the node after and before the one
// selected, in the order the document is drawn in.
type (
	ActionMoveNext struct{}
	ActionMovePrev struct{}
)

func (ActionMoveNext) isAction() {}
func (ActionMovePrev) isAction() {}

// ActionMoveIn asks to go one step towards the leaves: to unfold the selected
// container, or to select the first thing inside one already open.
//
// ActionMoveOut is its opposite: fold the selected container, or select what
// holds it. The pair is named for the direction taken through the tree rather
// than for the keys pressed, because which of the two things it does depends
// on what is selected.
type (
	ActionMoveIn  struct{}
	ActionMoveOut struct{}
)

func (ActionMoveIn) isAction()  {}
func (ActionMoveOut) isAction() {}

// ActionMoveFirst and ActionMoveLast ask for the first and the last node of
// the document.
//
// The last node, not the last row: the rows closing everything still open come
// after it, and the cursor does not land on those.
type (
	ActionMoveFirst struct{}
	ActionMoveLast  struct{}
)

func (ActionMoveFirst) isAction() {}
func (ActionMoveLast) isAction()  {}

// ActionScrollHalfDown and ActionScrollHalfUp ask to read on by half a screen,
// carrying the cursor along with the text.
type (
	ActionScrollHalfDown struct{}
	ActionScrollHalfUp   struct{}
)

func (ActionScrollHalfDown) isAction() {}
func (ActionScrollHalfUp) isAction()   {}

// ActionScrollBy asks for the window to move, rather than for the selection
// to. Rows is positive downwards.
//
// It is what a wheel produces. How far one turn reaches is a habit of the
// device and of the terminal, so it arrives as a distance rather than as a
// request this layer would have to put a number to.
type ActionScrollBy struct{ Rows int }

func (ActionScrollBy) isAction() {}

// ActionExpandAll and ActionCollapseAll ask for the whole document to be
// unfolded, or for it to be folded down to an overview of its shape.
type (
	ActionExpandAll   struct{}
	ActionCollapseAll struct{}
)

func (ActionExpandAll) isAction()   {}
func (ActionCollapseAll) isAction() {}

// ActionToggleView asks for the document to be shown the other way.
//
// It names the step rather than the destination, because there are two views
// and one key between them. What is being asked for is "the other one", which
// is what the key on the keyboard means.
type ActionToggleView struct{}

func (ActionToggleView) isAction() {}

// ActionEdit asks for the selected node to be edited.
//
// What editing one means is decided where the document is, because it depends
// on what is selected: a value to type over, a boolean to flip, a container to
// fold. One key says "act on this"; the answers are not several keys.
type ActionEdit struct{}

func (ActionEdit) isAction() {}

// ActionRenameKey asks for the key of the selected member to be changed. Only
// a member of an object has one.
type ActionRenameKey struct{}

func (ActionRenameKey) isAction() {}

// ActionAddChild asks for a new value at the end of the selected container.
// ActionAddSibling asks for one immediately after the selected node in its
// parent. Both enter the same insertion flow once that destination is known.
type (
	ActionAddChild   struct{}
	ActionAddSibling struct{}
)

func (ActionAddChild) isAction()   {}
func (ActionAddSibling) isAction() {}

// ActionDelete asks for the selected node and everything beneath it to be
// removed. The document root is not a node this action can remove.
type ActionDelete struct{}

func (ActionDelete) isAction() {}

// ActionChangeType asks for the selected node to become a value of another
// type, carrying over what can be carried.
type ActionChangeType struct{}

func (ActionChangeType) isAction() {}

// ActionPromptChange reports what has been typed so far, so that an answer
// which cannot be committed says so while it is being typed rather than when
// Enter is pressed.
//
// The text arrives with the report instead of being held here: the widget
// collecting it belongs to the layer that owns the terminal, and a copy on
// this side is a copy that could differ from what is on screen.
type ActionPromptChange struct{ Text string }

func (ActionPromptChange) isAction() {}

// ActionPromptSubmit is Enter on a prompt being typed into: the answer, whole.
type ActionPromptSubmit struct{ Text string }

func (ActionPromptSubmit) isAction() {}

// ActionPromptChoose is a key pressed on a prompt that offers keys.
//
// It carries the key rather than the meaning, because what the keys mean is
// written on the prompt and is therefore the prompt's to say: the layer that
// drew "[s] string" is the one that knows s asked for a string.
type ActionPromptChoose struct{ Key rune }

func (ActionPromptChoose) isAction() {}

// ActionCancel is Esc: the edit in progress is dropped, however many answers
// it had gathered.
type ActionCancel struct{}

func (ActionCancel) isAction() {}

// ActionSave asks for the document to be written to the file it came from.
//
// It carries no path. Where a document is saved is where it was opened, and a
// request that named somewhere else would be a different operation with a
// different question to ask about the file already there.
type ActionSave struct{}

func (ActionSave) isAction() {}

// ActionUndo and ActionRedo ask for the version of the document before the
// last change, and for the one after it.
//
// They are not the opposite of any particular edit. A version is a whole tree,
// so both of these are the same request — make another version current — asked
// in two directions.
type (
	ActionUndo struct{}
	ActionRedo struct{}
)

func (ActionUndo) isAction() {}
func (ActionRedo) isAction() {}

// ActionResize reports how many rows the document can be drawn in.
//
// It is the one Action that does not come from a key. Resizing a window is
// still something the person did, and the layer that owns the terminal is
// still the one translating it, so it arrives the same way everything else
// does. Height is the room left for the document, not the height of the
// terminal: subtracting the status bar, and later an inspector, is a decision
// that belongs to whoever lays the screen out.
type ActionResize struct{ Height int }

func (ActionResize) isAction() {}

// Effect is work the application cannot do itself and hands back to the
// presentation layer, which owns the terminal.
//
// Do returns effects instead of acting, so that a transition can be checked
// by looking at what it asked for rather than at what happened to the
// screen.
type Effect interface{ isEffect() }

// EffectQuit asks the program to stop.
type EffectQuit struct{}

func (EffectQuit) isEffect() {}

// EffectBeginInput asks for a text box holding Text, ready to be typed in.
//
// It is an Effect rather than a field of PromptInfo because seeding happens
// once, when the prompt appears, while PromptInfo is read on every redraw. A
// field would have to be paired with something telling the drawing side that
// this is a new prompt rather than the same one a keystroke later.
//
// Text is the value in full, however much of it a row had room for: what is
// edited is the document, not the abbreviation of it that was on screen. It is
// spelled the way the document spells it, since a terminal can neither show
// nor take the characters JSON writes as escapes.
//
// OneLine is the same value with its line breaks spelled as escapes too. It is
// there for a box that cannot hold as many rows as the value has: what such a
// box would otherwise do is take the value as far as its own limit, leaving
// someone who only looked at a value able to commit it short. Both spellings
// are read back the same way, so a box may use either.
type EffectBeginInput struct {
	Text      string
	OneLine   string
	Multiline bool
}

func (EffectBeginInput) isEffect() {}
