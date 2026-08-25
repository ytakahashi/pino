package application

// flow is whatever the session is in the middle of, and nil when it is in the
// middle of nothing.
//
// One field of this type holds it, rather than one field per kind of flow.
// Separate fields could be set at once, and the one not being answered would
// still be there afterwards to be answered against another version of the
// document; a single field cannot hold two things.
//
// The interface and its methods are unexported, so the flows are the ones
// written in this package: no other layer can put a session into a state the
// rules here do not describe. A flow reads the session to say what it is
// asking — a confirmation counts the nodes it would discard — and never
// writes to it. What an answer does to the document happens where the answer
// is taken.
//
// A flow whose prompt returns PromptText must also implement textFlow. Without
// it, the flow can put an input box on screen whose text no flow reads.
type flow interface {
	// mode is which of the modes this flow puts the session in. Deriving it is
	// what makes "a confirmation with nothing to confirm" a state nobody can
	// write.
	mode() Mode

	// prompt is the question on screen while this flow is the one in progress.
	prompt(a *App) PromptInfo

	// choose takes a key pressed on that prompt. The flow that drew "[r]
	// Reload" is the one that knows r means reloading, so the keys offered
	// and the keys accepted are written in one place and cannot drift apart.
	// A key the prompt does not offer does nothing.
	choose(a *App, key rune) []Effect
}

// textFlow is a flow whose answer arrives as the whole contents of an input
// box. The terminal uses the same two Actions for every such box; the flow
// that put it on screen owns validating and accepting its text.
type textFlow interface {
	validate(a *App, text string)
	submit(a *App, text string)
}

var (
	_ textFlow = (*editFlow)(nil)
	_ textFlow = (*searchFlow)(nil)
)

func (a *App) validate(text string) {
	// Text Actions can only come from the input box a textFlow requested.
	// Ignore direct Actions aimed at another flow instead of letting an
	// unrelated prompt consume text it did not ask for.
	if f, ok := a.flow.(textFlow); ok {
		f.validate(a, text)
	}
}

func (a *App) submit(text string) {
	// Keep the same boundary as validate: terminal input cannot submit text
	// without a text box, and callers constructing Actions directly must not
	// route it into a choice or confirmation flow.
	if f, ok := a.flow.(textFlow); ok {
		f.submit(a, text)
	}
}
