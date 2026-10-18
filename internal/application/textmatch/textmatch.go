// Package textmatch decides whether a typed term matches a piece of text.
//
// It is a package of its own so that every screen which searches or narrows
// text by a typed term applies one rule, rather than a copy of it per screen
// that can drift apart. It depends on the standard library alone, so any of
// them can use it without depending on another.
//
// What an empty term means is left to the caller: it matches every text here,
// and a caller to whom an empty term means something else checks for it first.
package textmatch

import "strings"

// Query is a term and the comparison rule it selects.
//
// A term unchanged by lowercasing selects case-insensitive matching; otherwise
// matching is exact about case. The term itself decides the rule, so there is
// no separate option that can get out of step with what is shown to the user.
//
// The zero value behaves like New("").
type Query struct {
	text string
	fold bool
}

// New keeps text unchanged and selects its comparison rule.
func New(text string) Query {
	return Query{text: text, fold: strings.ToLower(text) == text}
}

// Text returns the original term.
func (q Query) Text() string { return q.text }

// Match reports whether text contains the term under its comparison rule.
// An empty term matches every text, including empty text; callers decide
// whether an empty term has a different meaning in their own context.
func (q Query) Match(text string) bool {
	if q.fold {
		text = strings.ToLower(text)
	}

	return strings.Contains(text, q.text)
}
