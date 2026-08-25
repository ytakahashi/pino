package application

import (
	"strings"

	"github.com/ytakahashi/pino/internal/domain"
)

const noMatches = "no matches"

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

// walkSearchNodes visits nodes in the order an expanded document draws them.
// key is empty for the root and array elements; an empty object key is
// indistinguishable here, but no non-empty search term can match either one.
// Returning false from visit stops the whole walk, not only the current
// subtree; hasHit relies on no node being visited after a match.
func walkSearchNodes(
	root domain.Node,
	visit func(path domain.Path, key string, node domain.Node) bool,
) {
	if root == nil {
		return
	}

	var walk func(domain.Node, domain.Path, string) bool
	walk = func(node domain.Node, path domain.Path, key string) bool {
		if !visit(path, key, node) {
			return false
		}

		switch node.Kind() {
		case domain.KindObject:
			for _, member := range node.(*domain.Object).All() {
				if !walk(
					member.Value,
					path.Child(domain.KeySegment(member.Key)),
					member.Key,
				) {
					return false
				}
			}

		case domain.KindArray:
			for i, element := range node.(*domain.Array).All() {
				if !walk(element, path.Child(domain.IndexSegment(i)), "") {
					return false
				}
			}

		case domain.KindString, domain.KindNumber, domain.KindBool, domain.KindNull:
			// Scalars have no children.
		}

		return true
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

// hasHit stops at the first match because prompt validation only needs to
// distinguish an empty result from a non-empty one.
func hasHit(root domain.Node, q query) bool {
	found := false
	walkSearchNodes(root, func(_ domain.Path, key string, node domain.Node) bool {
		if matchesNode(q, key, node) {
			found = true

			return false
		}

		return true
	})

	return found
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
	walkSearchNodes(root, func(path domain.Path, key string, node domain.Node) bool {
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

		return true
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

// searchFlow is a search term being typed. The accepted search stays in App
// until this flow submits a replacement, so cancelling can discard the input
// without reconstructing the previous search.
type searchFlow struct{ err string }

func (*searchFlow) mode() Mode { return ModeSearch }

func (f *searchFlow) prompt(*App) PromptInfo {
	return PromptInfo{Kind: PromptText, Title: "/", Error: f.err}
}

func (*searchFlow) choose(*App, rune) []Effect { return nil }

func (f *searchFlow) validate(a *App, text string) {
	q := newQuery(text)
	if q.isZero() || (a.doc != nil && hasHit(a.doc.Root(), q)) {
		f.err = ""

		return
	}

	f.err = noMatches
}

func (f *searchFlow) submit(a *App, text string) {
	q := newQuery(text)
	if q.isZero() {
		a.search = searchState{}
		a.flow = nil
		a.settle(a.render())

		return
	}

	if a.doc == nil {
		f.err = noMatches

		return
	}

	candidate := searchState{query: q}
	candidate.refresh(a.doc.Root(), a.view.Cursor)
	if len(candidate.hits) == 0 {
		f.err = noMatches

		return
	}

	// Accepting a search includes a match at the current cursor. Navigation
	// differs deliberately: n and N always leave the current match.
	target := a.view.Cursor
	if !candidate.on {
		// There is at least one hit, so next cannot fail.
		target, _ = candidate.next()
	}

	a.search = candidate
	a.flow = nil
	a.moveToSearchHit(target)
}

// beginSearch asks for a fresh term. It deliberately seeds an empty input:
// accepting that input clears the search, while cancelling leaves the last
// accepted term untouched.
func (a *App) beginSearch() []Effect {
	if a.doc == nil || a.flow != nil {
		return nil
	}

	a.flow = &searchFlow{}

	return []Effect{EffectBeginInput{}}
}

func (a *App) searchNext() {
	if a.flow != nil {
		return
	}

	target, ok := a.search.next()
	if ok {
		a.moveToSearchHit(target)
	}
}

func (a *App) searchPrev() {
	if a.flow != nil {
		return
	}

	target, ok := a.search.prev()
	if ok {
		a.moveToSearchHit(target)
	}
}

func (a *App) moveToSearchHit(target domain.Path) {
	a.view.Reveal(target)
	a.view.Cursor = target
	a.settle(a.render())
}
