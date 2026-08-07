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
