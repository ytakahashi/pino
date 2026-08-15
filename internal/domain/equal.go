package domain

import "slices"

// Equal reports whether a and b are the same document: the same values, in
// the same order, spelled the same way, with the same comments around them.
//
// It is what pino asks before writing a file. The document is encoded, the
// bytes are parsed again, and the tree that comes back is compared with the
// one on screen; a defect in the encoder is then found while the file on disk
// is still untouched. Proving that needs a comparison of what two trees hold,
// which is what sameValue deliberately is not: that one answers whether
// replacing a single node would change anything, and settles a container by
// identity because the caller is trying to save a keystroke's worth of work.
//
// Numbers are compared by their literal. 1.50 and 1.5 denote the same
// quantity but not the same document, and writing the second where the file
// said the first is exactly the kind of silent rewriting this check exists to
// catch.
//
// Trivia is compared although strict JSON leaves it empty everywhere, so that
// an encoder which one day drops a comment is caught by the check that is
// already in the save path rather than by the user reading the diff.
//
// Both roots must be nodes. A nil, or a Node holding a nil pointer, is a
// mistake in the caller rather than something a document can hold, and it is
// refused before anything else happens: two of them are equal by pointer, and
// answering "these documents are the same" for a value that is missing would
// let a save go ahead on the strength of it.
//
// Only the roots are checked. Everything below one is there already, because
// NewObject and NewArray refuse a member or an element without a value, so
// the walk is free to assume that what it reaches is real — the same
// assumption the renderer and the edits make.
func Equal(a, b Node) bool {
	if isNilNode(a) || isNilNode(b) {
		panic("domain: cannot compare a node that is not there")
	}

	return equalNodes(a, b)
}

// equalNodes compares two nodes of a tree, each of which is known to be one.
//
// Two identical pointers are answered before anything is read, which is why a
// subtree shared between two versions of a document costs nothing to compare.
func equalNodes(a, b Node) bool {
	if a == b {
		return true
	}

	if a.Kind() != b.Kind() {
		return false
	}

	// Switching on Kind rather than on the concrete type so that a kind added
	// later is reported here by the exhaustive linter. A missing case would
	// make two documents compare equal without either being read, which is
	// the one answer this function must never give wrongly.
	switch a.Kind() {
	case KindObject:
		return equalObjects(a.(*Object), b.(*Object))

	case KindArray:
		return equalArrays(a.(*Array), b.(*Array))

	case KindString:
		return a.(*String).Value() == b.(*String).Value() &&
			equalTrivia(a.Trivia(), b.Trivia())

	case KindNumber:
		return a.(*Number).Raw() == b.(*Number).Raw() &&
			equalTrivia(a.Trivia(), b.Trivia())

	case KindBool:
		return a.(*Bool).Value() == b.(*Bool).Value() &&
			equalTrivia(a.Trivia(), b.Trivia())

	case KindNull:
		// There is only one null, however many nodes spell it.
		return equalTrivia(a.Trivia(), b.Trivia())

	default:
		// Unreachable while Kind and the concrete types are set together.
		// Refusing is the safe answer: the caller is deciding whether it is
		// safe to overwrite a file.
		return false
	}
}

// equalObjects compares members pairwise, in order.
//
// Order is part of the document: pino writes members back in the order it
// read them, so two objects holding the same pairs in a different order are
// two different files.
func equalObjects(a, b *Object) bool {
	if a.Len() != b.Len() {
		return false
	}

	for i, m := range a.All() {
		other := b.At(i)

		// The key and the comments around the pair belong to the member
		// rather than to the value, so neither is reached by the recursion.
		if m.Key != other.Key || !equalTrivia(m.Trivia, other.Trivia) {
			return false
		}

		if !equalNodes(m.Value, other.Value) {
			return false
		}
	}

	return equalTrivia(a.Trivia(), b.Trivia())
}

func equalArrays(a, b *Array) bool {
	if a.Len() != b.Len() {
		return false
	}

	for i, e := range a.All() {
		if !equalNodes(e, b.At(i)) {
			return false
		}
	}

	return equalTrivia(a.Trivia(), b.Trivia())
}

// equalTrivia compares the comments two nodes or members carry.
//
// A Comment holds a string and a bool, so the slices compare element by
// element with ==. Trivia itself does not, which is why this exists at all:
// the slices are unexported and copied on construction, and reading them here
// is what the same package is for.
func equalTrivia(a, b Trivia) bool {
	return slices.Equal(a.before, b.before) && slices.Equal(a.after, b.after)
}
