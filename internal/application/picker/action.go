package picker

// Action is a request from the user to the picker, already stripped of how it
// was made. The presentation layer maps keys to these, as it does for the
// session's own Actions.
type Action interface{ isAction() }

// ActionMoveNext and ActionMovePrev ask for the name below and above the one
// selected. ActionMoveFirst and ActionMoveLast ask for the first and the last.
type (
	ActionMoveNext  struct{}
	ActionMovePrev  struct{}
	ActionMoveFirst struct{}
	ActionMoveLast  struct{}
)

func (ActionMoveNext) isAction()  {}
func (ActionMovePrev) isAction()  {}
func (ActionMoveFirst) isAction() {}
func (ActionMoveLast) isAction()  {}

// ActionScrollHalfDown and ActionScrollHalfUp ask to read on by half a window,
// carrying the selection along with the names.
type (
	ActionScrollHalfDown struct{}
	ActionScrollHalfUp   struct{}
)

func (ActionScrollHalfDown) isAction() {}
func (ActionScrollHalfUp) isAction()   {}

// ActionScrollBy asks for the window to move by Rows, positive downwards,
// taking the selection along only as far as keeping it in the window needs.
type ActionScrollBy struct{ Rows int }

func (ActionScrollBy) isAction() {}

// ActionBeginFilter asks for a term to narrow the list by.
//
// ActionFilterChange reports the term as typed so far, and the list narrows
// with every report. ActionFilterSubmit keeps the term and returns to the
// list; ActionFilterCancel drops what was typed and puts back the term and the
// selection there were before typing began.
//
// The text arrives with each report instead of being held here, because the
// widget collecting it belongs to the layer that owns the terminal.
type (
	ActionBeginFilter  struct{}
	ActionFilterChange struct{ Text string }
	ActionFilterSubmit struct{ Text string }
	ActionFilterCancel struct{}
)

func (ActionBeginFilter) isAction()  {}
func (ActionFilterChange) isAction() {}
func (ActionFilterSubmit) isAction() {}
func (ActionFilterCancel) isAction() {}

// ActionClearFilter asks for every candidate back, dropping the term in effect.
type ActionClearFilter struct{}

func (ActionClearFilter) isAction() {}

// ActionOpen asks for the selected file to be opened.
type ActionOpen struct{}

func (ActionOpen) isAction() {}

// ActionShowHelp asks for the list of what the keys do, and ActionCloseHelp
// for the list of files back.
type (
	ActionShowHelp  struct{}
	ActionCloseHelp struct{}
)

func (ActionShowHelp) isAction()  {}
func (ActionCloseHelp) isAction() {}

// ActionQuit asks to leave pino. It is honoured in every mode, since nothing in
// the picker can be lost by leaving.
type ActionQuit struct{}

func (ActionQuit) isAction() {}

// Effect is work the picker hands back to the presentation layer.
type Effect interface{ isEffect() }

// EffectOpen asks for the file at Path to be opened.
//
// The picker does not open it: it knows nothing of documents. Whoever opens it
// reports a failure back through Picker.Failed.
type EffectOpen struct{ Path string }

func (EffectOpen) isEffect() {}

// EffectQuit asks the program to stop.
type EffectQuit struct{}

func (EffectQuit) isEffect() {}

// EffectBeginInput asks for a text box holding Text, ready to be typed in.
type EffectBeginInput struct{ Text string }

func (EffectBeginInput) isEffect() {}
