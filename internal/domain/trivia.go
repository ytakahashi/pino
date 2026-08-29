package domain

import (
	"iter"
	"strconv"
	"strings"
)

// Comment is a single validated comment attached to a node or a member.
// Its text does not include the // or /* */ delimiters.
//
// It holds only value types, so handing one out by copy cannot expose the
// document to modification.
type Comment struct {
	text    string
	block   bool
	ownLine bool
}

// InvalidCommentError reports text that cannot be safely written as a
// comment.
type InvalidCommentError struct {
	Text   string
	Reason string
	cause  error
}

func (e *InvalidCommentError) Error() string {
	return "invalid comment " + strconv.Quote(e.Text) + ": " + e.Reason
}

// Unwrap returns the underlying validation error when one carries details
// beyond the comment-specific reason.
func (e *InvalidCommentError) Unwrap() error { return e.cause }

// NewComment builds a comment after checking that its delimiters cannot be
// escaped and expose text as JSON syntax.
func NewComment(text string, block, ownLine bool) (Comment, error) {
	fail := func(reason string, cause error) (Comment, error) {
		return Comment{}, &InvalidCommentError{Text: text, Reason: reason, cause: cause}
	}

	if err := checkUTF8(text); err != nil {
		return fail("invalid UTF-8", err)
	}

	if block {
		if strings.Contains(text, "*/") {
			return fail("block comment contains */", nil)
		}
	} else if strings.ContainsAny(text, "\r\n") {
		return fail("line comment contains a newline", nil)
	}

	return Comment{text: text, block: block, ownLine: ownLine}, nil
}

// Text returns the text between the comment delimiters.
func (c Comment) Text() string { return c.text }

// Block reports whether the comment uses /* */ rather than //.
func (c Comment) Block() bool { return c.block }

// OwnLine reports whether the comment starts on a line of its own.
func (c Comment) OwnLine() bool { return c.ownLine }

// Trivia carries the comments surrounding a node or a member.
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
	inside []Comment
}

// NewTrivia builds Trivia from comments before and after an item, plus
// comments inside a container after its last child. All slices are copied.
func NewTrivia(before, after, inside []Comment) Trivia {
	return Trivia{
		before: append([]Comment(nil), before...),
		after:  append([]Comment(nil), after...),
		inside: append([]Comment(nil), inside...),
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

// Inside iterates over comments between a container's last child and its
// closing delimiter, in document order.
func (t Trivia) Inside() iter.Seq[Comment] {
	return func(yield func(Comment) bool) {
		for _, c := range t.inside {
			if !yield(c) {
				return
			}
		}
	}
}

// IsEmpty reports whether there is no comment to render or to preserve.
func (t Trivia) IsEmpty() bool {
	return len(t.before) == 0 && len(t.after) == 0 && len(t.inside) == 0
}
