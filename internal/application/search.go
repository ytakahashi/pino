package application

import (
	"strings"

	"github.com/ytakahashi/pino/internal/domain"
)

// query is a search term and the comparison rule it selects.
//
// A term without uppercase letters selects case-insensitive matching. The
// term itself decides the rule, so there is no separate option that can get
// out of step with what is shown to the user.
type query struct {
	text string
	fold bool
}

func newQuery(text string) query {
	return query{text: text, fold: strings.ToLower(text) == text}
}

func (q query) isZero() bool { return q.text == "" }

func (q query) match(text string) bool {
	// An empty term clears the search. strings.Contains would instead make it
	// match every node, so keep that session-level meaning true at this lower
	// boundary as well.
	if q.isZero() {
		return false
	}

	if q.fold {
		text = strings.ToLower(text)
	}

	return strings.Contains(text, q.text)
}

// walkSearchNodes visits every node in the order an expanded document draws
// them. key is empty for the root and array elements; an empty object key is
// indistinguishable here, but no non-empty search term can match either one.
func walkSearchNodes(
	root domain.Node,
	visit func(path domain.Path, key string, node domain.Node),
) {
	if root == nil {
		return
	}

	var walk func(domain.Node, domain.Path, string)
	walk = func(node domain.Node, path domain.Path, key string) {
		visit(path, key, node)

		switch node.Kind() {
		case domain.KindObject:
			for _, member := range node.(*domain.Object).All() {
				walk(
					member.Value,
					path.Child(domain.KeySegment(member.Key)),
					member.Key,
				)
			}

		case domain.KindArray:
			for i, element := range node.(*domain.Array).All() {
				walk(element, path.Child(domain.IndexSegment(i)), "")
			}

		case domain.KindString, domain.KindNumber, domain.KindBool, domain.KindNull:
			// Scalars have no children.
		}
	}

	walk(root, domain.Path{}, "")
}

// hits returns matching node paths in expanded drawing order. It searches the
// document rather than rendered lines, so folding, string elision and view
// punctuation cannot change the result.
func hits(root domain.Node, q query) []domain.Path {
	state := searchState{query: q}
	state.refresh(root, domain.Path{})

	return state.hits
}

func matchesNode(q query, key string, node domain.Node) bool {
	if q.match(key) {
		return true
	}

	switch node.Kind() {
	case domain.KindObject, domain.KindArray:
		return false

	case domain.KindString:
		return q.match(node.(*domain.String).Value())

	case domain.KindNumber:
		return q.match(node.(*domain.Number).Raw())

	case domain.KindBool:
		if node.(*domain.Bool).Value() {
			return q.match("true")
		}

		return q.match("false")

	case domain.KindNull:
		return q.match("null")

	default:
		// Unreachable while Kind and the concrete node types are extended
		// together in domain.
		panic("application: cannot search node of kind " + node.Kind().String())
	}
}

// searchState keeps the current term and values derived from it. The derived
// values are rebuilt from the current root instead of being adjusted after
// edits, so undo, redo and reload need no separate reconciliation rules.
type searchState struct {
	query query

	hits   []domain.Path
	passed int
	on     bool
}

// refresh rebuilds the result and the cursor's position in it in one tree
// walk. Counting only the hits cannot locate a cursor that is not itself a
// hit, which is why cursor passage is observed while every node is visited.
// The cursor is expected to resolve in root; if it does not, it is treated as
// following every hit. App must therefore settle the cursor before refreshing.
func (s *searchState) refresh(root domain.Node, cursor domain.Path) {
	s.hits = nil
	s.passed = 0
	s.on = false

	if root == nil || s.query.isZero() {
		return
	}

	beforeCursor := true
	walkSearchNodes(root, func(path domain.Path, key string, node domain.Node) {
		atCursor := path.Equal(cursor)
		if matchesNode(s.query, key, node) {
			s.hits = append(s.hits, path)

			switch {
			case atCursor:
				s.on = true
			case beforeCursor:
				s.passed++
			}
		}

		if atCursor {
			beforeCursor = false
		}
	})
}

func (s *searchState) next() (domain.Path, bool) {
	if len(s.hits) == 0 {
		return domain.Path{}, false
	}

	index := s.passed
	if s.on {
		index++
	}

	return s.hits[index%len(s.hits)], true
}

func (s *searchState) prev() (domain.Path, bool) {
	if len(s.hits) == 0 {
		return domain.Path{}, false
	}

	index := (s.passed - 1 + len(s.hits)) % len(s.hits)

	return s.hits[index], true
}
