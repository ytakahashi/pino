package domain

import "strconv"

// Resolve returns the node that p addresses within root.
//
// It reports false when the path leads nowhere: a key the object does not
// have, a position past the end of an array, or a step taken from a value
// that has no children. A path built by walking a document always resolves;
// one that came from outside, or one held since before an edit, may not.
//
// Segments are matched by their reference token rather than by their kind,
// for the same reason Path.Equal compares that way: a pointer parsed from
// text cannot tell an array index from an object key spelled alike, and only
// the document can. A path of pure key segments, which is all ParsePointer
// can produce, therefore resolves into arrays as well.
func Resolve(root Node, p Path) (Node, bool) {
	// A session with nothing open has no node to offer, whichever path is
	// asked for. This is the only place a missing node can enter: the
	// constructors keep one out of a tree, so every node reached below is
	// really there and needs no check of its own.
	if isNilNode(root) {
		return nil, false
	}

	n := root

	for _, seg := range p.All() {
		// The switch is on Kind rather than on the concrete type so that a
		// kind added later is reported here by the exhaustive linter instead
		// of quietly resolving to nothing, which is the wrong answer if what
		// was added holds children. domain sets the two together, so the
		// assertions below cannot fail.
		switch n.Kind() {
		case KindObject:
			m, ok := n.(*Object).Lookup(seg.Token())
			if !ok {
				return nil, false
			}

			n = m.Value

		case KindArray:
			a := n.(*Array)

			// The token has to be the plain spelling of the position. RFC 6901
			// allows neither a leading zero nor a sign, and Atoi on its own
			// would read "01" and "+1" as elements the pointer does not name.
			i, err := strconv.Atoi(seg.Token())
			if err != nil || i < 0 || i >= a.Len() || strconv.Itoa(i) != seg.Token() {
				return nil, false
			}

			n = a.At(i)

		case KindString, KindNumber, KindBool, KindNull:
			// A value with no children, with the path still going on.
			return nil, false

		default:
			return nil, false
		}
	}

	return n, true
}
