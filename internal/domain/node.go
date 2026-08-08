// Package domain holds pino's representation of a JSON document and the
// pure operations over it. It depends on nothing outside the standard
// library: parsing lives in infrastructure, rendering in application.
//
// The tree is immutable. Editing produces a new root by copying only the
// nodes along the changed path, so untouched subtrees are shared. Undo and
// redo then reduce to swapping a root pointer, and an unchanged subtree can
// be recognised by pointer identity.
//
// Every node type is therefore a pointer type. Comparing two Node values
// with == compares the interface, which is safe for pointers but would panic
// for a value type containing a slice or a map. Node is sealed so that this
// holds for every implementation, not just the ones defined here.
package domain

import (
	"iter"
	"strconv"
	"unicode/utf8"
)

// Kind identifies the JSON type of a Node.
type Kind uint8

const (
	KindObject Kind = iota
	KindArray
	KindString
	KindNumber
	KindBool
	KindNull
)

func (k Kind) String() string {
	switch k {
	case KindObject:
		return "object"
	case KindArray:
		return "array"
	case KindString:
		return "string"
	case KindNumber:
		return "number"
	case KindBool:
		return "boolean"
	case KindNull:
		return "null"
	default:
		return "unknown"
	}
}

// Node is a JSON value. The implementations are *Object, *Array, *String,
// *Number, *Bool and *Null.
//
// Implementations expose no setters: a node is never modified once built.
//
// The interface is sealed by an unexported method, so it cannot be implemented
// outside this package. JSON has a closed set of types, and more importantly
// an outside implementation could be a value type holding a slice, which would
// turn the == used to detect unsaved changes and unchanged subtrees into a
// runtime panic.
type Node interface {
	Kind() Kind
	Trivia() Trivia
	isNode()
}

// isNilNode reports whether n refers to no node at all.
//
// A Node holding a nil pointer does not compare equal to nil: the interface
// carries a type as well as a value, so n != nil is true while any method
// touching a field panics. Kind is the one method that does not, which is why
// it can be asked here.
//
// Such a node cannot come from a document — a JSON null parses to a *Null —
// so it only ever arrives from a mistake in pino. The constructors refuse it
// (see NewObject), and everything walking a tree is then free to assume that
// what it reaches is really there.
func isNilNode(n Node) bool {
	if n == nil {
		return true
	}

	// Switching on Kind rather than on the concrete type so that a kind added
	// later is reported here by the exhaustive linter. A case missing from
	// this list would let a nil of the new type into a tree, which is the bug
	// this function exists to prevent.
	switch n.Kind() {
	case KindObject:
		return n.(*Object) == nil
	case KindArray:
		return n.(*Array) == nil
	case KindString:
		return n.(*String) == nil
	case KindNumber:
		return n.(*Number) == nil
	case KindBool:
		return n.(*Bool) == nil
	case KindNull:
		return n.(*Null) == nil
	default:
		// Unreachable while Kind and the concrete types are set together.
		// Rejecting is the safe answer for the callers, all of which are
		// deciding whether to admit n into a tree.
		return true
	}
}

// Member is one key/value pair of an Object.
//
// Its fields are exported because a Member is handed out by value, so
// assigning to them affects only the copy. The Node it refers to is shared,
// and remains immutable.
type Member struct {
	Key    string
	Value  Node
	Trivia Trivia
}

// Object is a JSON object. Keys are unique and their order is preserved;
// see DuplicateKeyError.
type Object struct {
	members []Member
	index   map[string]int
	trivia  Trivia
}

// DuplicateKeyError reports two members of the same object sharing a key.
//
// pino keeps "every path in the document is unique" as an invariant, because
// the cursor, the collapsed set, search and undo all identify nodes by path.
// The member indexes let the caller map the conflict back to a position in
// the source.
type DuplicateKeyError struct {
	Key   string
	First int // index of the first member with this key
	Dup   int // index of the offending member
}

func (e *DuplicateKeyError) Error() string {
	return "duplicate key " + strconv.Quote(e.Key)
}

// InvalidUTF8Error reports text that is not valid UTF-8.
//
// RFC 8259 requires JSON to be encoded in UTF-8, so such bytes are not a
// document pino can write back. They are refused where they would enter the
// tree rather than replaced with U+FFFD: substituting would rewrite the bytes
// of a file the user only asked to look at, and the loss would surface at
// save time with nothing left to trace it to.
//
// Refusing here also lets everything downstream — measuring a string's width,
// searching it, encoding it — take valid UTF-8 for granted.
type InvalidUTF8Error struct {
	// Text is the offending string, which is how a parser finds the value or
	// the key it came from in order to report a position.
	Text string

	// Index is the byte offset of the first sequence that could not be
	// decoded.
	Index int
}

func (e *InvalidUTF8Error) Error() string {
	return "invalid UTF-8 at byte " + strconv.Itoa(e.Index)
}

// checkUTF8 reports the first byte of s that is not part of a valid UTF-8
// sequence.
func checkUTF8(s string) error {
	for i, r := range s {
		if r != utf8.RuneError {
			continue
		}

		// U+FFFD is a character in its own right and may legitimately appear
		// in a document. Only a decode one byte wide is a failed one.
		if _, size := utf8.DecodeRuneInString(s[i:]); size == 1 {
			return &InvalidUTF8Error{Text: s, Index: i}
		}
	}

	return nil
}

// NewObject builds an Object from members, in order.
//
// It returns a *DuplicateKeyError if two members share a key, and an
// *InvalidUTF8Error if a key is not valid UTF-8. Keys are checked here rather
// than by a constructor of their own, because Member is a plain struct and
// this is the point where one enters a document.
//
// It panics if a member has no value. A document cannot ask for that — a JSON
// null is a *Null — so it is a mistake in the caller rather than something the
// input can contain, which is the same line IndexSegment draws. Returning an
// error instead would spread handling for an impossible case through every
// caller, while letting one through would leave a hole in the tree that
// panics much later, in whichever walk reaches it first.
//
// The slice is copied, so later changes by the caller do not reach the
// object. Copying the slice alone is enough because a Member holds only a
// string, an immutable Node and an immutable Trivia.
func NewObject(members []Member) (*Object, error) {
	index := make(map[string]int, len(members))
	for i, m := range members {
		if err := checkUTF8(m.Key); err != nil {
			return nil, err
		}

		if isNilNode(m.Value) {
			panic("domain: no value for object key " + strconv.Quote(m.Key))
		}

		if first, dup := index[m.Key]; dup {
			return nil, &DuplicateKeyError{Key: m.Key, First: first, Dup: i}
		}

		index[m.Key] = i
	}

	return &Object{
		members: append([]Member(nil), members...),
		index:   index,
	}, nil
}

func (o *Object) isNode()        {}
func (o *Object) Kind() Kind     { return KindObject }
func (o *Object) Trivia() Trivia { return o.trivia }
func (o *Object) Len() int       { return len(o.members) }

// At returns the i-th member. It panics if i is out of range.
func (o *Object) At(i int) Member { return o.members[i] }

// All iterates over the members in document order.
func (o *Object) All() iter.Seq2[int, Member] {
	return func(yield func(int, Member) bool) {
		for i, m := range o.members {
			if !yield(i, m) {
				return
			}
		}
	}
}

// Lookup returns the member with the given key.
func (o *Object) Lookup(key string) (Member, bool) {
	i, ok := o.index[key]
	if !ok {
		return Member{}, false
	}

	return o.members[i], true
}

// IndexOf returns the position of the member with the given key.
func (o *Object) IndexOf(key string) (int, bool) {
	i, ok := o.index[key]

	return i, ok
}

// Array is a JSON array.
type Array struct {
	elements []Node
	trivia   Trivia
}

// NewArray builds an Array from elements, in order. The slice is copied.
//
// It panics if an element is missing, for the reason given on NewObject.
func NewArray(elements []Node) *Array {
	for i, e := range elements {
		if isNilNode(e) {
			panic("domain: no value for array element " + strconv.Itoa(i))
		}
	}

	return &Array{elements: append([]Node(nil), elements...)}
}

func (a *Array) isNode()        {}
func (a *Array) Kind() Kind     { return KindArray }
func (a *Array) Trivia() Trivia { return a.trivia }
func (a *Array) Len() int       { return len(a.elements) }

// At returns the i-th element. It panics if i is out of range.
func (a *Array) At(i int) Node { return a.elements[i] }

// All iterates over the elements in order.
func (a *Array) All() iter.Seq2[int, Node] {
	return func(yield func(int, Node) bool) {
		for i, n := range a.elements {
			if !yield(i, n) {
				return
			}
		}
	}
}

// String is a JSON string. The value is unescaped: escaping is applied when
// encoding, and is never shown to the user.
type String struct {
	value  string
	trivia Trivia
}

// NewString wraps a string value, which must be valid UTF-8.
//
// It returns an *InvalidUTF8Error otherwise. The check is here, rather than
// left to whoever writes the document out, so that a file pino cannot encode
// is refused while there is still a position to report it against, and so
// that text pasted into an edit is caught as it is entered.
func NewString(v string) (*String, error) {
	if err := checkUTF8(v); err != nil {
		return nil, err
	}

	return &String{value: v}, nil
}

func (s *String) isNode()        {}
func (s *String) Kind() Kind     { return KindString }
func (s *String) Trivia() Trivia { return s.trivia }
func (s *String) Value() string  { return s.value }

// Number is a JSON number, kept as the literal text of the source.
//
// Decoding into a float64 would lose precision and notation for large
// integers, exponents and trailing zeros, so a number that is never edited is
// written back byte for byte.
type Number struct {
	raw    string
	trivia Trivia
}

// NewNumber wraps a numeric literal. raw must be valid JSON number syntax;
// callers obtain it either from the parser or from validated user input.
func NewNumber(raw string) *Number { return &Number{raw: raw} }

func (n *Number) isNode()        {}
func (n *Number) Kind() Kind     { return KindNumber }
func (n *Number) Trivia() Trivia { return n.trivia }
func (n *Number) Raw() string    { return n.raw }

// Bool is a JSON true or false.
type Bool struct {
	value  bool
	trivia Trivia
}

func NewBool(v bool) *Bool { return &Bool{value: v} }

func (b *Bool) isNode()        {}
func (b *Bool) Kind() Kind     { return KindBool }
func (b *Bool) Trivia() Trivia { return b.trivia }
func (b *Bool) Value() bool    { return b.value }

// Null is the JSON null.
type Null struct {
	trivia Trivia
}

func NewNull() *Null { return &Null{} }

func (n *Null) isNode()        {}
func (n *Null) Kind() Kind     { return KindNull }
func (n *Null) Trivia() Trivia { return n.trivia }
