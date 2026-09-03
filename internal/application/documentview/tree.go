package documentview

import (
	"strconv"

	"github.com/ytakahashi/pino/internal/domain"
)

// NewTreeRenderer returns the renderer that lays a document out as a tree,
// which is how pino shows the shape of a document rather than its text.
func NewTreeRenderer() Renderer { return treeRenderer{} }

// treeRenderer draws one document with one set of options.
//
// The options sit on the renderer for the reason they do on jsonRenderer: they
// say how to draw rather than what, so what is left in the arguments of the
// recursion is exactly the subtree and where it sits.
type treeRenderer struct{ opt Options }

// Markers say which rows can be opened, and which of those already are.
//
// They are spans rather than something the presentation layer adds from Kind
// and Collapsed, so that a dump of the rows shows what the screen shows. The
// trailing space is part of the marker: it makes the marker two cells wide,
// which is one level of the tree, so a child's name sits under its parent's.
const (
	markerExpanded = "▼ "
	markerFolded   = "▶ "
)

// rootLabel names the document root.
//
// The root is a member of nothing, so it has neither a key nor an index to be
// named after. What is left is the pointer that addresses it, which RFC 6901
// writes as the empty string and which reads as "/" — the same reading the
// status bar gives it.
const rootLabel = "/"

// Render draws root. It returns nil when no document is open.
//
// The receiver is unused, as it is on jsonRenderer: the options arrive with
// the call, so the renderer that draws is built here.
func (treeRenderer) Render(root domain.Node, opt Options) []Line {
	if root == nil {
		return nil
	}

	r := treeRenderer{opt: opt}
	lines := appendComments(nil, root.Trivia().Before(), domain.Path{}, 0)
	return append(lines, r.node(root, domain.Path{}, 0, rootLabel, false)...)
}

// node returns the rows for the subtree at n.
//
// Composing a subtree's rows into its parent's, rather than appending to one
// shared slice, keeps every recursive step a plain "subtree to rows" function,
// which is what a cache keyed on the immutable node pointer would need later.
//
// It takes one argument fewer than the JSON view's: nothing here has to know
// whether n is the last of its siblings, because a tree has no commas between
// its rows. label is the name drawn at the head of the row — the key of an
// object member, the position of an array element, or the root's own name.
func (r treeRenderer) node(n domain.Node, p domain.Path, depth int, label string, drawBefore bool) []Line {
	var before []Line
	var inline []Span
	if drawBefore {
		before, inline = commentsBefore(n.Trivia().Before(), p, depth)
	}
	var lines []Line

	// The switch is on Kind rather than on the concrete type so that a kind
	// added later is reported here by the exhaustive linter instead of
	// silently falling through to the scalar branch. domain sets the two
	// together, so the assertions below cannot fail.
	switch n.Kind() {
	case domain.KindObject:
		o := n.(*domain.Object)

		head, expanded := r.head(p, depth, label, "{", "}", o.Len(), o.Trivia().HasInside(), inline)
		if !expanded {
			lines = []Line{head}
			break
		}

		// One row for each member at least, plus the head.
		lines = make([]Line, 0, o.Len()+1)
		lines = append(lines, head)

		for _, m := range o.All() {
			childPath := p.Child(domain.KeySegment(m.Key))
			lines = appendComments(lines, m.Trivia.Before(), childPath, depth+1)
			lines = append(lines, r.node(m.Value, childPath, depth+1, m.Key, true)...)
			lines = appendOwnedComments(lines, m.Trivia.After(), childPath, depth+1)
		}

		lines = appendComments(lines, o.Trivia().Inside(), p, depth+1)

	case domain.KindArray:
		a := n.(*domain.Array)

		head, expanded := r.head(p, depth, label, "[", "]", a.Len(), a.Trivia().HasInside(), inline)
		if !expanded {
			lines = []Line{head}
			break
		}

		lines = make([]Line, 0, a.Len()+1)
		lines = append(lines, head)

		for i, e := range a.All() {
			// An element is named by its position. That reads as a key would,
			// which is why the badge on this row says the container is an
			// array: without it "0: ..." would not say which of the two it is.
			childPath := p.Child(domain.IndexSegment(i))
			lines = appendComments(lines, e.Trivia().Before(), childPath, depth+1)
			lines = append(lines, r.node(e, childPath, depth+1, strconv.Itoa(i), false)...)
		}

		lines = appendComments(lines, a.Trivia().Inside(), p, depth+1)

	case domain.KindString, domain.KindNumber, domain.KindBool, domain.KindNull:
		// The colon is what separates a row holding a value from a row to be
		// opened: those carry a badge in its place.
		spans := []Span{treeName(label), punct(": ")}
		spans = append(spans, inline...)
		lines = []Line{{
			Path:  p,
			Kind:  LineSingle,
			Depth: depth,
			Spans: append(spans, ScalarSpan(n, r.opt.MaxStrLen)),
		}}

	default:
		panic("documentview: cannot render node of kind " + n.Kind().String())
	}

	lines = append(before, lines...)
	return appendOwnedComments(lines, n.Trivia().After(), p, depth)
}

// head is the row a container is drawn on, and whether its children follow it.
//
// The three answers it gives are the three the JSON view gives, kind for kind:
// a container with nothing in it and one folded away are drawn on a single row
// (a folded one is a LineSingle carrying the flag, not a LineOpen, so that an
// open row always has a close row to match in the view that has them), and
// only an open container is a LineOpen with rows beneath it. Being empty wins
// over being folded, since there is nothing to hide and a row that offers to
// unfold into nothing is a worse answer than the brackets themselves.
//
// Keeping that agreement is what lets h and l mean the same thing in both
// views: they read Kind and Collapsed, and never ask which renderer drew.
func (r treeRenderer) head(
	p domain.Path, depth int, label, left, right string, n int, inside bool, inline []Span,
) (Line, bool) {
	spans := []Span{treeName(label)}
	spans = append(spans, inline...)
	spans = append(spans, badge(left, right, n))

	if n == 0 && !inside {
		// No marker: a row that offers to open would be offering nothing.
		return Line{
			Path:  p,
			Kind:  LineSingle,
			Depth: depth,
			Spans: spans,
		}, false
	}

	if isCollapsed(r.opt, p) {
		return Line{
			Path:      p,
			Kind:      LineSingle,
			Depth:     depth,
			Spans:     append([]Span{guide(markerFolded)}, spans...),
			Collapsed: true,
		}, false
	}

	return Line{
		Path:  p,
		Kind:  LineOpen,
		Depth: depth,
		Spans: append([]Span{guide(markerExpanded)}, spans...),
	}, true
}

// treeName is how a member is named in the tree: bare where the name reads as
// itself, quoted where it does not.
//
// Escaping is the test because it answers both questions at once. A key that
// escaping leaves alone is safe to put on screen as it stands and is written
// the same way when the document is saved; one it changes holds a quote, a
// backslash or a control character, and printing that raw would split the row
// or hand an escape sequence to the terminal. The empty key is the one name
// escaping leaves alone that still cannot be drawn bare, since the row would
// begin with the colon and name nothing.
//
// Array positions and the root come through here as well. Neither ever needs
// quoting, so naming stays one rule rather than three.
func treeName(label string) Span {
	if label == "" || domain.EscapeString(label) != label {
		return Span{Text: domain.QuoteString(label), Role: RoleKey}
	}

	return Span{Text: label, Role: RoleKey}
}

// badge is the pair of brackets that says what a container is, with the number
// of children between them when there are any.
//
// It says two things in one place. The brackets keep object and array apart,
// which the rows below cannot do on their own: {"0": "a"} and ["a"] both draw
// a child named 0. The count makes an empty container the same shape as any
// other rather than a form of its own, and gives editing a structure something
// visible to move.
//
// The space in front belongs to the badge: a label and its badge are always
// neighbours, so nothing is gained by making the caller place it.
func badge(left, right string, n int) Span {
	if n == 0 {
		return punct(" " + left + right)
	}

	return punct(" " + left + strconv.Itoa(n) + right)
}

func guide(text string) Span { return Span{Text: text, Role: RoleTreeGuide} }
