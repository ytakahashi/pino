package application

import (
	"iter"
	"maps"

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

// Next is the view Tab switches to.
//
// Switching is a step through the views rather than a choice among them: there
// are two, and one key moves between them. A third would arrive as an action
// naming the view it wants instead, since stepping through three in a fixed
// order is not how anyone would ask for the one they mean.
func (v ViewMode) Next() ViewMode {
	switch v {
	case ViewJSON:
		return ViewTree

	case ViewTree:
		return ViewJSON
	}

	// Not reached: the switch covers every view, and the linter keeps it so.
	return ViewJSON
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

// Apply follows an edit: what it took out is dropped, what moved is moved, and
// the cursor goes where the edit says.
//
// The three steps answer three different ways a folded set goes stale, and the
// order between them is part of the answer.
//
//  1. What the edit removed is dropped. This cannot be worked out by asking
//     which paths still resolve: taking an element out of an array moves the
//     next one into its place, so the path of what was deleted goes on
//     resolving, to a node nobody folded.
//  2. What moved is moved, every path being looked at once against the whole
//     set of renames. Steps 1 and 2 both read paths as the document spelled
//     them before the edit, which is why removal comes first: afterwards, a
//     path that has just shifted into the place of a deleted one would be
//     dropped in its stead.
//  3. Whatever the new tree does not have is dropped, which is Retain.
//
// Step 3 should find nothing once the first two have run. It is here so that
// the invariant below holds by construction rather than by trusting every edit
// to have declared what it removed, and it is the same code undo goes through.
//
// The invariant: every path in Collapsed resolves in the current root.
func (v *ViewState) Apply(r domain.EditResult) {
	moved := make(map[string]struct{}, len(v.Collapsed))

	for pointer := range v.Collapsed {
		if domain.PointerRemoved(r.Removed, pointer) {
			continue
		}

		moved[domain.RewritePointer(r.Renames, pointer)] = struct{}{}
	}

	// The contents are replaced rather than the map, since the renderer is
	// handed this very map on every redraw.
	clear(v.Collapsed)
	maps.Copy(v.Collapsed, moved)

	v.Retain(r.Root)

	// The edit is the only thing that knows where it happened, so where to
	// stand is its answer and not something worked out from the rows. Whether
	// that place is on screen is settled afterwards, by whoever draws.
	v.Cursor = r.Cursor
}

// Retain drops whatever the document no longer has.
//
// Undo and redo need this without an edit to follow, since a version is a
// whole tree rather than a change: nothing says which paths stopped being
// there, only which tree is current now.
//
// Undo does not put folds back. What is folded is how the document is being
// looked at rather than what it contains, and undo restores what it contains.
// A fold left over an array whose elements have shifted therefore lands one
// place along, which one keystroke corrects.
func (v *ViewState) Retain(root domain.Node) {
	for pointer := range v.Collapsed {
		p, err := domain.ParsePointer(pointer)
		if err != nil {
			// Not reachable: the set is keyed by what Path.String produced.
			// Dropping is the safe answer for a key nothing can resolve.
			delete(v.Collapsed, pointer)

			continue
		}

		if _, ok := domain.Resolve(root, p); !ok {
			delete(v.Collapsed, pointer)
		}
	}
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
