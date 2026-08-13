package domain

import "testing"

// describe is a node written the way a test can compare it, which for the
// values Convert produces is the value itself. Containers only ever come back
// empty, so their contents need no spelling out.
func describe(t *testing.T, n Node) string {
	t.Helper()

	switch n.Kind() {
	case KindObject:
		if n.(*Object).Len() == 0 {
			return "{}"
		}

		return "{...}"

	case KindArray:
		if n.(*Array).Len() == 0 {
			return "[]"
		}

		return "[...]"

	case KindString:
		return QuoteString(n.(*String).Value())

	case KindNumber:
		return n.(*Number).Raw()

	case KindBool:
		if n.(*Bool).Value() {
			return "true"
		}

		return "false"

	case KindNull:
		return "null"

	default:
		t.Fatalf("describe: unknown kind %v", n.Kind())

		return ""
	}
}
