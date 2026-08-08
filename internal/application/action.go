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

// ActionExpandAll and ActionCollapseAll ask for the whole document to be
// unfolded, or for it to be folded down to an overview of its shape.
type (
	ActionExpandAll   struct{}
	ActionCollapseAll struct{}
)

func (ActionExpandAll) isAction()   {}
func (ActionCollapseAll) isAction() {}

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
