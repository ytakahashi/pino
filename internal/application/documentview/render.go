package documentview

import (
	"strconv"

	"github.com/ytakahashi/pino/internal/domain"
)

// This file holds what every renderer has to draw the same way.
//
// Sharing is kept to that. Composing rows is where the views differ, and
// pulling that together would leave the difference between them expressed as
// flags and branches inside one builder. What is here instead is the handful
// of decisions that must not diverge: which nodes get a row at all, and how a
// value is written once it does.

// isCollapsed reports whether the container at p is folded away.
//
// Every renderer asks this and nothing else to decide whether a subtree is
// drawn, which is what makes the views agree on the rows the cursor can land
// on: the same document and the same folded set yield the same nodes, in the
// same depth-first order, whichever renderer is drawing. Only the shape of
// those rows differs.
//
// The set is keyed by JSON Pointer, so asking costs building one. Nothing is
// folded in a document just opened, and that is also when every row would pay,
// so an empty set is answered without touching a path at all.
func isCollapsed(opt Options, p domain.Path) bool {
	if len(opt.Collapsed) == 0 {
		return false
	}

	_, ok := opt.Collapsed[p.String()]

	return ok
}

// ScalarSpan is the drawn form of a value that occupies no rows of its own.
//
// It is shared so that what is on screen is what would be saved (the escaping
// rules are the encoder's) in every view at once, and so that a value shortened
// in one view is shortened at the same place in the other: a value that changed
// its spelling when the view changed would make the ellipsis read as decoration
// rather than as a cut.
//
// It is exported for the same reason: the inspector shows the selected value
// in full, and a pane that spelled a value its own way would disagree with the
// row it is describing.
func ScalarSpan(n domain.Node, maxLen int) Span {
	switch v := n.(type) {
	case *domain.String:
		return stringSpan(v.Value(), maxLen)

	case *domain.Number:
		// The literal as it was written: a number is shown the way the file
		// spells it, exponents and trailing zeros included.
		return Span{Text: v.Raw(), Role: RoleNumberValue}

	case *domain.Bool:
		return Span{Text: strconv.FormatBool(v.Value()), Role: RoleBoolValue}

	case *domain.Null:
		return Span{Text: "null", Role: RoleNullValue}

	default:
		panic("documentview: cannot render node of kind " + n.Kind().String())
	}
}

// stringSpan is the drawn form of a string value, shortened to maxLen runes if
// it is longer than that. A maxLen of zero or less draws the value in full.
//
// A shortened value ends in an ellipsis inside its quotes. The mark is not
// decoration: everywhere else what is on screen is exactly what would be
// saved, which is why the renderer and the encoder share one set of escaping
// rules, and this is the one place that departs from it. Without the mark a
// value would look as though it ended where the row does.
//
// Only values are shortened, never keys. A shortened key would leave the row
// naming a member that the pointer in the status bar does not, and keys are
// short in the documents people edit by hand.
//
// Length is counted in runes rather than in the width the terminal gives them,
// which needs a table this layer would have to take a dependency for. What is
// being avoided here is one value filling the screen, not a row overflowing
// it: rows are cut to the width of the terminal where they are drawn.
func stringSpan(v string, maxLen int) Span {
	if maxLen > 0 {
		if head, cut := truncateRunes(v, maxLen); cut {
			// Escaped before the quotes go on, so that the ellipsis lands
			// inside them. Cutting the escaped form instead could split a \u
			// sequence and put half of it on screen.
			return Span{Text: `"` + domain.EscapeString(head) + `…"`, Role: RoleStringValue}
		}
	}

	// Quoted the way the document would be written, so that a control
	// character in a value is shown as an escape rather than sent to the
	// terminal.
	return Span{Text: domain.QuoteString(v), Role: RoleStringValue}
}

// truncateRunes returns the first n runes of s, and whether anything was left
// behind. Ranging over a string yields the byte offset of each rune, so the
// cut is found without counting the whole of a long value first.
func truncateRunes(s string, n int) (string, bool) {
	count := 0

	for i := range s {
		if count == n {
			return s[:i], true
		}

		count++
	}

	return s, false
}

func punct(text string) Span { return Span{Text: text, Role: RolePunct} }
