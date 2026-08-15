package documentview

import (
	"github.com/ytakahashi/pino/internal/domain"
)

// NewJSONRenderer returns the renderer that lays a document out as formatted
// JSON, which is how pino shows a file by default.
func NewJSONRenderer() Renderer { return jsonRenderer{} }

// jsonRenderer draws one document with one set of options.
//
// The options sit on the renderer rather than among the arguments of the
// recursion below: they are the same for every row, and they say how to draw
// rather than what. What is left in the arguments is exactly the subtree and
// where it sits, which is what a cache keyed on the node pointer would need.
type jsonRenderer struct{ opt Options }

// Render draws root. It returns nil when no document is open.
//
// The receiver is unused: the options arrive with the call, so the renderer
// that draws is built here rather than being the one Render was called on.
func (jsonRenderer) Render(root domain.Node, opt Options) []Line {
	if root == nil {
		return nil
	}

	// The root has no key in front of it and no sibling to be separated
	// from, which is what a nil label and a final position mean.
	return jsonRenderer{opt: opt}.node(root, domain.Path{}, 0, nil, true)
}

// node returns the rows for the subtree at n.
//
// Composing a subtree's rows into its parent's, rather than appending to one
// shared slice, keeps every recursive step a plain "subtree to rows"
// function. That is what a cache keyed on the immutable node pointer would
// need later, and it is why nothing here reads state outside its arguments.
//
// label is drawn before the value on the first row: the key of an object
// member, or nothing for an array element or the root. last says whether n is
// the final child of its parent, which decides the separating comma.
func (r jsonRenderer) node(n domain.Node, p domain.Path, depth int, label []Span, last bool) []Line {
	// The switch is on Kind rather than on the concrete type so that a kind
	// added later is reported here by the exhaustive linter instead of
	// silently falling through to the scalar branch. domain sets the two
	// together, so the assertions below cannot fail.
	switch n.Kind() {
	case domain.KindObject:
		return r.object(n.(*domain.Object), p, depth, label, last)

	case domain.KindArray:
		return r.array(n.(*domain.Array), p, depth, label, last)

	case domain.KindString, domain.KindNumber, domain.KindBool, domain.KindNull:
		return []Line{{
			Path:  p,
			Kind:  LineSingle,
			Depth: depth,
			Spans: separated(spansOf(label, ScalarSpan(n, r.opt.MaxStrLen)), last),
		}}

	default:
		panic("documentview: cannot render node of kind " + n.Kind().String())
	}
}

// object returns the rows for o, opening and closing braces included.
//
// An empty object is drawn on a single row: an open and a close row with
// nothing between them would cost two rows to say what "{}" says in one, and
// leave a close row the cursor cannot land on directly below its open row.
//
// An object that is folded away is drawn on a single row too. It is a
// LineSingle rather than a LineOpen carrying the flag, so that an open row
// always has a close row to match: folding, deleting a subtree and caching one
// all want to treat the two as a pair. Being empty wins over being folded,
// since there is nothing to hide and a row that offers to unfold into nothing
// is a worse answer than the braces themselves.
func (r jsonRenderer) object(o *domain.Object, p domain.Path, depth int, label []Span, last bool) []Line {
	if o.Len() == 0 {
		return []Line{{
			Path:  p,
			Kind:  LineSingle,
			Depth: depth,
			Spans: separated(spansOf(label, punct("{}")), last),
		}}
	}

	if isCollapsed(r.opt, p) {
		return []Line{{
			Path:      p,
			Kind:      LineSingle,
			Depth:     depth,
			Spans:     separated(spansOf(label, punct("{…}")), last),
			Collapsed: true,
		}}
	}

	// One row for each member at least, plus the braces.
	lines := make([]Line, 0, o.Len()+2)
	lines = append(lines, Line{
		Path:  p,
		Kind:  LineOpen,
		Depth: depth,
		Spans: spansOf(label, punct("{")),
	})

	for i, m := range o.All() {
		lines = append(lines, r.node(
			m.Value,
			p.Child(domain.KeySegment(m.Key)),
			depth+1,
			memberLabel(m.Key),
			i == o.Len()-1,
		)...)
	}

	// The closing row carries the path of the node it closes, so that folding
	// can find it and so that a cursor on the node knows where its rows end.
	return append(lines, Line{
		Path:  p,
		Kind:  LineClose,
		Depth: depth,
		Spans: separated([]Span{punct("}")}, last),
	})
}

// array returns the rows for a, brackets included. An empty array is drawn on
// a single row, and so is a folded one, both for the reasons given on object.
func (r jsonRenderer) array(a *domain.Array, p domain.Path, depth int, label []Span, last bool) []Line {
	if a.Len() == 0 {
		return []Line{{
			Path:  p,
			Kind:  LineSingle,
			Depth: depth,
			Spans: separated(spansOf(label, punct("[]")), last),
		}}
	}

	if isCollapsed(r.opt, p) {
		return []Line{{
			Path:      p,
			Kind:      LineSingle,
			Depth:     depth,
			Spans:     separated(spansOf(label, punct("[…]")), last),
			Collapsed: true,
		}}
	}

	lines := make([]Line, 0, a.Len()+2)
	lines = append(lines, Line{
		Path:  p,
		Kind:  LineOpen,
		Depth: depth,
		Spans: spansOf(label, punct("[")),
	})

	for i, e := range a.All() {
		lines = append(lines, r.node(
			e,
			p.Child(domain.IndexSegment(i)),
			depth+1,
			nil,
			i == a.Len()-1,
		)...)
	}

	return append(lines, Line{
		Path:  p,
		Kind:  LineClose,
		Depth: depth,
		Spans: separated([]Span{punct("]")}, last),
	})
}

// memberLabel is the key and separator drawn in front of an object member.
func memberLabel(key string) []Span {
	return []Span{
		{Text: domain.QuoteString(key), Role: RoleKey},
		{Text: ": ", Role: RolePunct},
	}
}

// spansOf puts a row's label in front of what follows it.
//
// It always allocates, so that a label built for one row cannot be reached
// through the spans of another. Appending in place would write into the
// backing array whenever it happened to have room, which is the same trap
// Path.Child avoids by copying.
func spansOf(label []Span, rest ...Span) []Span {
	spans := make([]Span, 0, len(label)+len(rest)+1)
	spans = append(spans, label...)

	return append(spans, rest...)
}

// separated ends a row with the comma that divides it from its next sibling.
func separated(spans []Span, last bool) []Span {
	if last {
		return spans
	}

	return append(spans, punct(","))
}
