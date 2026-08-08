package application

import (
	"iter"

	"github.com/ytakahashi/pino/internal/domain"
)

// ViewMode selects which renderer draws the document.
type ViewMode uint8

const (
	ViewJSON ViewMode = iota
	ViewTree
)

// String returns the label shown in the status bar.
func (v ViewMode) String() string {
	switch v {
	case ViewJSON:
		return "JSON"
	case ViewTree:
		return "TREE"
	default:
		return "UNKNOWN"
	}
}

// ViewState is how the document is being looked at, as opposed to what the
// document is.
//
// It survives a view switch untouched: the folded set and the cursor are
// shared by both renderers, so toggling between them does not move the
// selection or re-expand what the user has folded away.
type ViewState struct {
	// Collapsed holds the JSON Pointer of every folded node. Empty means
	// everything is expanded, which is where a document starts.
	Collapsed map[string]struct{}

	Cursor   domain.Path
	ViewMode ViewMode
	Scroll   int

	// MaxStrLen is how much of a long string value is shown, in runes.
	MaxStrLen int
}

// defaultMaxStrLen is how much of a string value is shown before the rest is
// left out.
//
// It is a plain number rather than something derived from the width of the
// terminal, which would move the cut as a window is resized and make what is
// on screen depend on how the screen is shaped. On a narrow terminal a deeply
// nested row will still be cut on the right, but that cut belongs to the
// display and leaves no mark; this one does.
//
// Unexported because nothing chooses it yet. An option to would arrive as a
// field on ViewState set from the command line, which is why the field is
// there and the renderer reads it rather than this constant.
const defaultMaxStrLen = 64

// NewViewState is a fully expanded document seen in the JSON view, with the
// cursor at the root.
func NewViewState() ViewState {
	return ViewState{
		Collapsed: make(map[string]struct{}),
		MaxStrLen: defaultMaxStrLen,
	}
}

// RenderOptions is the part of the view state a renderer needs.
//
// The map is handed over rather than copied: a renderer only reads it, and
// rendering happens on every redraw.
func (v ViewState) RenderOptions() RenderOptions {
	return RenderOptions{Collapsed: v.Collapsed, MaxStrLen: v.MaxStrLen}
}

// Collapse folds the node at p away. It reports whether anything changed, so
// that a caller knows whether the rows have to be produced again.
//
// The set is keyed by the text of a path rather than by the path itself, since
// a Path holds a slice and cannot be a map key. Every method here goes through
// that conversion so that no other part of the layer has to know of it.
func (v *ViewState) Collapse(p domain.Path) bool {
	pointer := p.String()
	if _, folded := v.Collapsed[pointer]; folded {
		return false
	}

	v.Collapsed[pointer] = struct{}{}

	return true
}

// Expand unfolds the node at p, reporting whether anything changed.
func (v *ViewState) Expand(p domain.Path) bool {
	pointer := p.String()
	if _, folded := v.Collapsed[pointer]; !folded {
		return false
	}

	delete(v.Collapsed, pointer)

	return true
}

// IsCollapsed reports whether the node at p is folded away.
func (v *ViewState) IsCollapsed(p domain.Path) bool {
	_, folded := v.Collapsed[p.String()]

	return folded
}

// CollapseAll folds every container of the document away, leaving the members
// of the root on screen.
//
// The root itself stays open. Folding it would leave one row reading "{…}",
// which is not an overview of a document but the absence of one, and taking in
// the shape of what is open is the whole point of asking for this.
//
// Containers inside the ones being folded are folded too, so unfolding one
// reveals its members already closed. That is what vim does when all folds are
// closed at once, and it is what makes unfolding a way of descending a level
// at a time.
func (v *ViewState) CollapseAll(root domain.Node) {
	for pointer := range collapsibleUnder(root) {
		v.Collapsed[pointer] = struct{}{}
	}
}

// ExpandAll unfolds the whole document.
//
// The set is emptied rather than replaced, since the renderer is handed this
// very map on every redraw.
func (v *ViewState) ExpandAll() {
	clear(v.Collapsed)
}

// collapsibleUnder yields the pointer of every container below root that has
// something in it. The root is not among them, and neither are containers with
// no members: folding either says nothing that leaving it open does not.
//
// The tree is walked rather than the rows, because folding a container hides
// what is inside it: a single pass over what is drawn would only ever reach
// the outermost level, and repeating until nothing changes would be a slower
// way of doing what one walk does.
func collapsibleUnder(root domain.Node) iter.Seq[string] {
	return func(yield func(string) bool) {
		var walk func(n domain.Node, p domain.Path) bool

		walk = func(n domain.Node, p domain.Path) bool {
			// The switch is on Kind rather than on the concrete type so that a
			// kind added later is reported here by the exhaustive linter: one
			// that holds children would otherwise be left open by this.
			switch n.Kind() {
			case domain.KindObject:
				o := n.(*domain.Object)
				if o.Len() == 0 {
					return true
				}

				if !p.IsRoot() && !yield(p.String()) {
					return false
				}

				for _, m := range o.All() {
					if !walk(m.Value, p.Child(domain.KeySegment(m.Key))) {
						return false
					}
				}

			case domain.KindArray:
				a := n.(*domain.Array)
				if a.Len() == 0 {
					return true
				}

				if !p.IsRoot() && !yield(p.String()) {
					return false
				}

				for i, e := range a.All() {
					if !walk(e, p.Child(domain.IndexSegment(i))) {
						return false
					}
				}

			case domain.KindString, domain.KindNumber, domain.KindBool, domain.KindNull:
				// Nothing to fold, and nothing below.
			}

			return true
		}

		walk(root, domain.Path{})
	}
}
