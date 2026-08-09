package application

import (
	"strings"

	"github.com/ytakahashi/pino/internal/domain"
)

// LineKind says what a line does to the structure around it.
//
// It is what lets the layers above treat a rendered document as a flat list
// while still respecting the tree: a row that can be opened is a LineOpen, a
// row that is folded away is a LineSingle carrying the flag, and the cursor
// skips LineClose.
//
// A close row always closes the nearest open row still waiting for one, and
// carries the same path. The converse does not hold: an open row need not have
// a close row, because that is a property of the view being drawn rather than
// of this model. The JSON view has closing braces to draw; the tree view has
// none, and a container there ends where the depth of the rows drops back.
//
// Requiring the converse would cost more than it is worth. The tree view would
// have to emit close rows holding nothing, which are blank lines on screen, or
// a Line would have to record which renderer made it. Nothing wants either:
// folding is the renderer declining to draw a subtree rather than anything
// counting rows in pairs, cursor movement only ever skips a close row, and an
// edit takes the extent of a subtree from the tree itself.
type LineKind uint8

const (
	LineSingle LineKind = iota // "port": 8080
	LineOpen                   // "server": {
	LineClose                  // }, which the cursor never lands on
)

func (k LineKind) String() string {
	switch k {
	case LineSingle:
		return "single"
	case LineOpen:
		return "open"
	case LineClose:
		return "close"
	default:
		return "unknown"
	}
}

// Role is what a piece of text is, not how it looks.
//
// The presentation layer owns the mapping from Role to a style. Keeping the
// colour out of the rendered line is what allows rendering to live in this
// layer without dragging a terminal library in with it, and it keeps the
// golden files that cover the layout free of escape sequences.
type Role uint8

const (
	RoleKey Role = iota
	RoleStringValue
	RoleNumberValue
	RoleBoolValue
	RoleNullValue
	RolePunct
	RoleTreeGuide // the arrows and rules drawn by the tree view
)

func (r Role) String() string {
	switch r {
	case RoleKey:
		return "key"
	case RoleStringValue:
		return "string"
	case RoleNumberValue:
		return "number"
	case RoleBoolValue:
		return "bool"
	case RoleNullValue:
		return "null"
	case RolePunct:
		return "punct"
	case RoleTreeGuide:
		return "guide"
	default:
		return "unknown"
	}
}

// Span is a run of text drawn in one style.
type Span struct {
	Text string
	Role Role
}

// Line is one rendered row of a document.
//
// Both views produce the same type, so cursor movement, scrolling, search
// highlighting and keeping the selection across a view switch are written
// once. Switching views is then swapping the Renderer and finding the row
// with the same Path.
//
// Spans hold the content of the row only: the leading indentation is not
// among them. Depth says how deep the row sits, and the presentation layer
// turns that into whitespace for the JSON view and into guides for the tree
// view. One row therefore renders correctly in both, and the indentation
// width stays a display decision.
type Line struct {
	Path      domain.Path
	Kind      LineKind
	Depth     int
	Spans     []Span
	Collapsed bool
}

// Text is the row without indentation and without styling.
func (l Line) Text() string {
	var b strings.Builder

	for _, s := range l.Spans {
		b.WriteString(s.Text)
	}

	return b.String()
}

// RenderOptions is what the view state contributes to rendering.
//
// Collapsed is keyed by JSON Pointer rather than by Path because a Path holds
// a slice and so cannot be a map key. The pointer is the canonical text form
// of a path, and paths within a document are unique, so the two identify a
// node equally well.
type RenderOptions struct {
	Collapsed map[string]struct{}
	MaxStrLen int // 0 means strings are shown in full
}

// Renderer turns a document into rows. The JSON view and the tree view are
// two implementations.
//
// It is an interface so that the naive full re-render can later be wrapped in
// a memoising one without anything above noticing.
type Renderer interface {
	Render(root domain.Node, opt RenderOptions) []Line
}
