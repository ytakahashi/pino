package domain

import (
	"iter"
	"strconv"
	"strings"
)

// Segment is one step from a node to one of its children: either a member of
// an object or an element of an array.
//
// Its fields are unexported so that a segment cannot be put into the
// contradictory state of naming both a key and an index. It remains
// comparable with ==.
type Segment struct {
	key   string
	index int
	isKey bool
}

// KeySegment addresses the member of an object with the given key.
func KeySegment(key string) Segment { return Segment{key: key, isKey: true} }

// IndexSegment addresses the i-th element of an array.
//
// It panics if i is negative. Array positions come from walking a document or
// from an edit working out an insertion point, so a negative one is a bug in
// the caller rather than bad input, and At panics on an out of range index for
// the same reason. Text that might carry a negative index, such as a pointer
// typed by the user, has to be checked against the array's length in any case,
// which this constructor cannot do.
//
// Left unchecked, a negative index would reach Array.At and panic much further
// from its origin, and Equal would quietly treat it as the object key "-1".
func IndexSegment(i int) Segment {
	if i < 0 {
		panic("domain: negative array index " + strconv.Itoa(i))
	}

	return Segment{index: i}
}

// IsKey reports whether the segment addresses an object member rather than an
// array element.
func (s Segment) IsKey() bool { return s.isKey }

// Key is the object key. It is meaningful only when IsKey reports true.
func (s Segment) Key() string { return s.key }

// Index is the array position. It is meaningful only when IsKey reports false.
func (s Segment) Index() int { return s.index }

// Token returns the RFC 6901 reference token for the segment, before escaping.
func (s Segment) Token() string {
	if s.isKey {
		return s.key
	}

	return strconv.Itoa(s.index)
}

// Path locates a node within a document.
//
// It is the single way pino identifies a node: the cursor, the collapsed set,
// search results and the position restored by undo are all paths. Every path
// in a document is unique, which is why duplicate object keys are rejected.
//
// The segments are unexported and Child always allocates, so a path can be
// stored and shared freely: no holder can reach into another one's segments,
// and appending to a parent cannot corrupt a sibling built from it.
//
// The zero value is the root.
type Path struct {
	segments []Segment
}

// Len is the depth of the path. Zero means the root.
func (p Path) Len() int { return len(p.segments) }

// IsRoot reports whether the path addresses the document root.
func (p Path) IsRoot() bool { return len(p.segments) == 0 }

// At returns the i-th segment. It panics if i is out of range.
func (p Path) At(i int) Segment { return p.segments[i] }

// All iterates over the segments from the root down.
func (p Path) All() iter.Seq2[int, Segment] {
	return func(yield func(int, Segment) bool) {
		for i, s := range p.segments {
			if !yield(i, s) {
				return
			}
		}
	}
}

// Child returns the path to a child of p. p is left untouched.
func (p Path) Child(seg Segment) Path {
	segments := make([]Segment, len(p.segments)+1)
	copy(segments, p.segments)
	segments[len(p.segments)] = seg

	return Path{segments: segments}
}

// Parent returns the path to the container holding p.
//
// The root is its own parent. Walking up therefore stops on IsRoot rather than
// on a second return value, which is what the callers want: recovering a lost
// cursor climbs until it finds a node that is still on screen, and the root
// always is.
//
// Unlike Child this shares its storage with p, which is safe precisely because
// Child does not: no method ever writes into a path's segments, so the only
// way to reach a shared array would be an append that never happens. Climbing
// from a deep path therefore allocates nothing.
func (p Path) Parent() Path {
	if len(p.segments) == 0 {
		return p
	}

	return Path{segments: p.segments[:len(p.segments)-1]}
}

// Equal reports whether p and q address the same node.
//
// Segments are compared by reference token rather than by kind, because a
// pointer parsed from text cannot tell an array index from an object key
// spelled the same way. In a well-formed document only one of the two is
// possible at a given position, so equal tokens mean the same node.
func (p Path) Equal(q Path) bool {
	if len(p.segments) != len(q.segments) {
		return false
	}

	for i, a := range p.segments {
		b := q.segments[i]
		if a == b {
			continue
		}

		if a.Token() != b.Token() {
			return false
		}
	}

	return true
}

// String returns the path as an RFC 6901 JSON Pointer.
//
// The root is the empty string, as the RFC requires. Presentation decides how
// to show that; "/" reads better in a status bar.
func (p Path) String() string {
	if len(p.segments) == 0 {
		return ""
	}

	var b strings.Builder

	for _, s := range p.segments {
		b.WriteByte('/')
		b.WriteString(escapeToken.Replace(s.Token()))
	}

	return b.String()
}

// RFC 6901 escaping. A single pass avoids the trap of rewriting the "0" that
// escaping "~" has just produced.
var (
	escapeToken   = strings.NewReplacer("~", "~0", "/", "~1")
	unescapeToken = strings.NewReplacer("~1", "/", "~0", "~")
)

// InvalidPointerError reports text that is not a well-formed JSON Pointer.
type InvalidPointerError struct {
	Pointer string
	Reason  string
}

func (e *InvalidPointerError) Error() string {
	return "invalid JSON Pointer " + strconv.Quote(e.Pointer) + ": " + e.Reason
}

// ParsePointer turns an RFC 6901 JSON Pointer into a Path.
//
// Every segment comes back as a key segment. A pointer on its own cannot say
// whether "/features/0" ends at the first element of an array or at the member
// "0" of an object, since both are legal JSON; telling them apart needs the
// document. Equal compares by token for that reason, and String round-trips
// either way.
func ParsePointer(s string) (Path, error) {
	if s == "" {
		return Path{}, nil
	}

	if s[0] != '/' {
		return Path{}, &InvalidPointerError{
			Pointer: s,
			Reason:  `a non-empty pointer must start with "/"`,
		}
	}

	// The leading "/" is a separator, not part of the first token, and a
	// trailing one denotes a final empty token: "/" addresses the member "".
	tokens := strings.Split(s[1:], "/")
	segments := make([]Segment, 0, len(tokens))

	for _, token := range tokens {
		if err := checkEscapes(s, token); err != nil {
			return Path{}, err
		}

		segments = append(segments, KeySegment(unescapeToken.Replace(token)))
	}

	return Path{segments: segments}, nil
}

// checkEscapes rejects a "~" that is not the start of "~0" or "~1". Without
// this the unescaper would silently pass such text through.
func checkEscapes(pointer, token string) error {
	for i := 0; i < len(token); i++ {
		if token[i] != '~' {
			continue
		}

		if i+1 >= len(token) {
			return &InvalidPointerError{
				Pointer: pointer,
				Reason:  `"~" at the end of a token must be escaped as "~0"`,
			}
		}

		switch token[i+1] {
		case '0', '1':
			i++ // consume the escape so "~01" is read as "~0" then "1"
		default:
			return &InvalidPointerError{
				Pointer: pointer,
				Reason:  `"~" must be followed by "0" or "1"`,
			}
		}
	}

	return nil
}
