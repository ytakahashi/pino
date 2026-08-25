package application

// Mode is what the next key press means.
//
// It is held by the application rather than the presentation layer because
// the transitions are part of the interaction rules, not of the drawing: a
// deletion opening a confirmation, or a cancelled edit returning to normal,
// are decisions that stay testable only while they are out of the terminal.
//
// The whole set is defined from the outset even though only ModeNormal is
// reachable so far, so that a switch over Mode is complete now and the
// exhaustive linter reports the ones that are not.
type Mode uint8

const (
	ModeNormal  Mode = iota
	ModeEdit         // editing a value or a key
	ModeInsert       // adding a member: key, then type, then value
	ModeConfirm      // confirming a deletion, a quit or an outside change
	ModeHelp
	ModeSearch // typing a search term
)

// String returns the label shown in the status bar.
func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeEdit:
		return "EDIT"
	case ModeInsert:
		return "INSERT"
	case ModeConfirm:
		return "CONFIRM"
	case ModeHelp:
		return "HELP"
	case ModeSearch:
		return "SEARCH"
	default:
		return "UNKNOWN"
	}
}
