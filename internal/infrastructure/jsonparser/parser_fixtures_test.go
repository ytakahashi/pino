package jsonparser

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// esc spells the JSON escape for a UTF-16 code unit.
//
// Fixtures build their escapes with this instead of writing them out, so that
// what reaches the parser is unmistakably the escape rather than the character
// it stands for. A fixture holding the character never reaches the code that
// reads escapes at all, and would go on passing if that code started refusing
// every escape it saw.
func esc(unit uint16) string { return fmt.Sprintf(`\u%04x`, unit) }

// escUpper is esc with the uppercase hexadecimal digits JSON also allows.
func escUpper(unit uint16) string { return fmt.Sprintf(`\u%04X`, unit) }

// dump renders a tree on one line, tagging each scalar with its kind and
// showing numbers as the text they were parsed from, so that a table can say
// what it expects without building a tree to compare against.
func dump(n domain.Node) string {
	var b strings.Builder

	writeNode(&b, n)

	return b.String()
}

func writeNode(b *strings.Builder, n domain.Node) {
	switch v := n.(type) {
	case *domain.Object:
		b.WriteByte('{')

		for i, m := range v.All() {
			if i > 0 {
				b.WriteByte(',')
			}

			b.WriteString(strconv.Quote(m.Key))
			b.WriteByte(':')
			writeNode(b, m.Value)
		}

		b.WriteByte('}')

	case *domain.Array:
		b.WriteByte('[')

		for i, e := range v.All() {
			if i > 0 {
				b.WriteByte(',')
			}

			writeNode(b, e)
		}

		b.WriteByte(']')

	case *domain.String:
		b.WriteByte('s')
		b.WriteString(strconv.Quote(v.Value()))

	case *domain.Number:
		b.WriteByte('n')
		b.WriteString(v.Raw())

	case *domain.Bool:
		b.WriteByte('b')
		b.WriteString(strconv.FormatBool(v.Value()))

	case *domain.Null:
		b.WriteString("null")

	default:
		b.WriteString("?")
	}
}

// parseStrict parses src as standard JSON and fails the test if it will not.
func parseStrict(t *testing.T, src string) domain.Node {
	t.Helper()

	node, err := New().Parse([]byte(src), domain.StrictJSON)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}

	return node
}

// syntaxErrorFor parses src expecting a *SyntaxError, and returns it.
func syntaxErrorFor(t *testing.T, src string, d domain.Dialect) *SyntaxError {
	t.Helper()

	node, err := New().Parse([]byte(src), d)
	if err == nil {
		t.Fatalf("Parse(%q) = %s, want an error", src, dump(node))
	}

	var syntaxErr *SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("Parse(%q) returned %T, want *SyntaxError", src, err)
	}

	return syntaxErr
}

// wantPosition checks where an error was reported.
func wantPosition(t *testing.T, err *SyntaxError, line, column int) {
	t.Helper()

	if err.Line != line || err.Column != column {
		t.Errorf("reported at %d:%d, want %d:%d (%s)", err.Line, err.Column, line, column, err.Msg)
	}
}
