package domain

import "iter"

// Comment is a single comment attached to a node or a member.
//
// It holds only value types, so handing one out by copy cannot expose the
// document to modification.
type Comment struct {
	Text  string
	Block bool // /* ... */ rather than // ...
}

// Trivia carries the comments surrounding a node or a member.
//
// pino only accepts standard JSON for now, so Trivia is always empty. The
// field exists from the outset because the tree is edited by path copying:
// adding a field later would mean auditing every copy site for one that
// forgets to carry it over, and a missed one silently drops the user's
// comments on edit.
//
// The slices are unexported and copied on construction, which makes a Trivia
// value immutable. That matters because Trivia travels inside Member, which
// is handed out by value: exported slice fields would let a caller reach the
// backing array of a node that is already part of the document.
//
// Trivia contains slices and is therefore not comparable with ==. Nodes are
// compared by pointer identity, so this does not affect them.
type Trivia struct {
	before []Comment
	after  []Comment
}

// NewTrivia builds Trivia from the comments on the lines above a node and the
// trailing comment on its own line. Both slices are copied.
func NewTrivia(before, after []Comment) Trivia {
	return Trivia{
		before: append([]Comment(nil), before...),
		after:  append([]Comment(nil), after...),
	}
}

// Before iterates over the comments on the lines above.
func (t Trivia) Before() iter.Seq[Comment] {
	return func(yield func(Comment) bool) {
		for _, c := range t.before {
			if !yield(c) {
				return
			}
		}
	}
}

// After iterates over the trailing comments on the same line.
func (t Trivia) After() iter.Seq[Comment] {
	return func(yield func(Comment) bool) {
		for _, c := range t.after {
			if !yield(c) {
				return
			}
		}
	}
}

// IsEmpty reports whether there is no comment to render or to preserve.
func (t Trivia) IsEmpty() bool {
	return len(t.before) == 0 && len(t.after) == 0
}
