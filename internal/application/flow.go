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
