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

// NewObject builds an Object from members, in order.
//
// It returns a *DuplicateKeyError if two members share a key. The slice is
// copied, so later changes by the caller do not reach the object. Copying the
// slice alone is enough because a Member holds only a string, an immutable
// Node and an immutable Trivia.
func NewObject(members []Member) (*Object, error) {
	index := make(map[string]int, len(members))
	for i, m := range members {
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
func NewArray(elements []Node) *Array {
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

func NewString(v string) *String { return &String{value: v} }

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
