package presentation

import "github.com/ytakahashi/pino/internal/application"

// How the screen is divided, in columns and rows.
const (
	// The smallest terminal pino draws in. Below either of these the screen
	// says so instead, which is a decision for whoever draws rather than for
	// the arithmetic here.
	minWidth  = 60
	minHeight = 10

	// At this width the inspector fits beside the tree without squeezing it.
	wideWidth = 100

	// The inspector is a fixed 32 columns rather than a share of the screen.
	// What it holds is a pointer, a type and a value, and none of them grow
	// with the terminal: a share would spend 60 columns of a 200 column screen
	// on the same words and take them from the document.
	inspectorWidth = 32

	// One row per field of the stacked pane: pointer, type, value or children,
	// the name within the parent, and the available editing keys.
	inspectorRows = 5

	statusBarRows = 1 // the strip along the bottom

	// The rule dividing the panes: a column of it beside the inspector, a row
	// of it above one.
	ruleWidth = 1
	ruleRows  = 1
)

// placement is where the inspector goes, if anywhere.
type placement uint8

const (
	placeNone placement = iota // the JSON view, which has no inspector
	placeSide
	placeBelow
)

func (p placement) String() string {
	switch p {
	case placeNone:
		return "none"
	case placeSide:
		return "side"
	case placeBelow:
		return "below"
	default:
		return "unknown"
	}
}

// layout is how the screen is divided. It is worked out from the size of the
// terminal and the view alone, so the arrangement is testable without a
// terminal and without a document.
type layout struct {
	// TooSmall reports that the terminal is below the size pino draws in. It
	// is a finding rather than a refusal: the fields below are filled in
	// either way, so that whoever draws chooses what to do about it and the
	// arithmetic here stays one thing.
	TooSmall bool

	BodyWidth  int
	BodyHeight int

	Inspector       placement
	InspectorWidth  int // placeSide
	InspectorHeight int // placeBelow, the rule counted

	// PromptHeight is the band between the document and the status bar, the
	// rule above it counted. It is zero whenever nothing is being asked.
	PromptHeight int
}

// layoutFor divides a terminal of the given size between the parts of the
// screen.
//
// It is a function of its arguments rather than a method on the model so that
// the boundaries, which are the whole of what it says, can be checked without
// building a terminal and reading a drawn screen back. The same judgement made
// clampScroll a function in the layer below.
//
// prompt is how many rows the band asking a question wants, which is worked out
// from what is on it rather than from the size of the screen. It arrives as a
// number so that this stays arithmetic: what the band holds is no business of
// how the screen is divided, and the boundaries below do not move when a
// question is asked.
//
// Every result is bounded below at zero. A terminal shorter than the parts
// asked of it is not an error to report but a screen with nothing left for the
// document, and rows of a negative height would be arithmetic nobody can draw.
func layoutFor(width, height int, view application.ViewMode, prompt int) layout {
	l := layout{
		TooSmall:   width < minWidth || height < minHeight,
		BodyWidth:  max(width, 0),
		BodyHeight: max(height-statusBarRows, 0),
	}

	// The band comes out of the screen before the inspector does. It is where
	// an answer is being typed, so a screen too short for both keeps the part
	// that is being used; the inspector describes the selection, which the
	// prompt is about to change anyway.
	l.PromptHeight = min(max(prompt, 0), l.BodyHeight)
	l.BodyHeight -= l.PromptHeight

	switch view {
	case application.ViewTree:
		if width >= wideWidth {
			l.Inspector = placeSide
			l.InspectorWidth = inspectorWidth
			l.BodyWidth = max(width-inspectorWidth-ruleWidth, 0)

			break
		}

		// Never more rows than are left once the bar has taken its own. The
		// pane wants six, and a terminal with fewer than that is one the
		// screen is about to say it cannot draw in; claiming six rows of
		// three would still be describing a screen nobody can lay out.
		l.Inspector = placeBelow
		l.InspectorHeight = min(inspectorRows+ruleRows, l.BodyHeight)
		l.BodyHeight -= l.InspectorHeight

	case application.ViewJSON:
		// No inspector. The status bar already names the pointer and the type
		// of the selection, which is what the pane would repeat here.
	}

	return l
}
