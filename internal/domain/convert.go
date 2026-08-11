package domain

import "strconv"

// Convert is v seen as kind k: the carried-over value where one can be read,
// and the zero value of k where it cannot.
//
// The pairs that carry a value over are the ones where the text of the value
// is the same in both types — a number and the string spelling it, a boolean
// and "true". Everything else yields the zero value of the target kind, which
// includes turning a container into a primitive: the children are lost, and
// the layer above says so before asking for the change.
//
// Converting to the kind v already has returns v itself, so that choosing the
// type a node already has is not an edit at all.
//
// It panics if k is not one of the six kinds, for the reason given at the
// bottom of the switch.
//
// No input reaches the error today: the text carried over is a number literal
// or a spelling of a boolean, both ASCII, and a string is only ever carried to
// a string, which the line above answers without rebuilding anything. The
// constructors are still allowed to refuse, because deciding here that they
// cannot would put the knowledge of what they check in the wrong place, and a
// kind added later may well carry text a caller typed.
func Convert(v Node, k Kind) (Node, error) {
	if v.Kind() == k {
		return v, nil
	}

	switch k {
	case KindString:
		s, err := NewString(asText(v))
		if err != nil {
			return nil, err
		}

		return s, nil

	case KindNumber:
		return asNumber(v), nil

	case KindBool:
		return NewBool(asBool(v)), nil

	case KindNull:
		return NewNull(), nil

	case KindObject:
		o, err := NewObject(nil)
		if err != nil {
			// Unreachable: an object with no members has no key to repeat and
			// none to decode.
			return nil, err
		}

		return o, nil

	case KindArray:
		return NewArray(nil), nil

	default:
		// k is an argument rather than something read out of a tree, so a
		// value outside the enum is a mistake in the caller and cannot come
		// from a document — the same line IndexSegment draws for a negative
		// index. Returning a node-less success instead would put nil where a
		// root or a child is about to go, and the panic would surface later,
		// in whichever rebuild reached it first.
		panic("domain: unknown kind " + strconv.Itoa(int(k)))
	}
}

// asText is v read as a string: the literal for a number, the spelling for a
// boolean, and the empty string where there is nothing to read.
func asText(v Node) string {
	switch v.Kind() {
	case KindNumber:
		// The source literal, not a reformatted one, so that 1.50 stays "1.50".
		return v.(*Number).Raw()

	case KindBool:
		if v.(*Bool).Value() {
			return "true"
		}

		return "false"

	case KindString:
		return v.(*String).Value()

	case KindNull, KindObject, KindArray:
		return ""

	default:
		return ""
	}
}

// asNumber is v read as a number: the value where the text spells one, and
// zero otherwise.
func asNumber(v Node) *Number {
	if v.Kind() == KindString {
		if n, err := ParseNumber(v.(*String).Value()); err == nil {
			return n
		}
	}

	// Text that is not a number is not an error here: the type was asked for,
	// and a node of that type is what comes back. A boolean yields zero rather
	// than one because JSON has no correspondence between the two — treating
	// true as 1 would be JavaScript's rule, not this format's.
	return NewNumber("0")
}

// asBool is v read as a boolean: true only where the text says so.
func asBool(v Node) bool {
	return v.Kind() == KindString && v.(*String).Value() == "true"
}

// CountDescendants is how many nodes would go with n, n itself not counted.
// It is what a confirmation says out loud before a subtree is discarded.
//
// The whole subtree is counted rather than the immediate children, because
// that is the number a reader needs in order to judge what is about to be
// lost: an object holding two objects holding ten members each loses twelve
// nodes, not two.
func CountDescendants(n Node) int {
	switch n.Kind() {
	case KindObject:
		o := n.(*Object)

		total := o.Len()
		for _, m := range o.All() {
			total += CountDescendants(m.Value)
		}

		return total

	case KindArray:
		a := n.(*Array)

		total := a.Len()
		for _, e := range a.All() {
			total += CountDescendants(e)
		}

		return total

	case KindString, KindNumber, KindBool, KindNull:
		return 0

	default:
		return 0
	}
}
