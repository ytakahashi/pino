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

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"object", `{"host":"localhost"}`, `{"host":s"localhost"}`},
		{"nested", "{\n  \"server\": {\n    \"port\": 8080\n  }\n}", `{"server":{"port":n8080}}`},
		{
			"every kind",
			`{"s":"x","n":1,"t":true,"f":false,"z":null,"a":[],"o":{}}`,
			`{"s":s"x","n":n1,"t":btrue,"f":bfalse,"z":null,"a":[],"o":{}}`,
		},
		{"empty object", `{}`, `{}`},
		{"empty array", `[]`, `[]`},
		{"array of objects", `[{"a":1},{"a":2}]`, `[{"a":n1},{"a":n2}]`},
		{"nested arrays", `[[1,[2]],[]]`, `[[n1,[n2]],[]]`},
		{"root string", `"x"`, `s"x"`},
		{"root number", `42`, `n42`},
		{"root true", `true`, `btrue`},
		{"root false", `false`, `bfalse`},
		{"root null", `null`, `null`},
		{"leading and trailing whitespace", "\n\t {\"a\": 1} \n", `{"a":n1}`},

		// Escapes are undone on the way in: the tree holds text, and the
		// escaping is reapplied when the document is written or drawn.
		{"short escapes", `["\"\\\/\b\f\n\r\t"]`, `[s"\"\\/\b\f\n\r\t"]`},
		{"unicode escape", `["` + esc(0x0041) + esc(0x00E9) + `"]`, `[s"Aé"]`},
		{"unicode escape in uppercase hex", `["` + escUpper(0x00E9) + `"]`, `[s"é"]`},
		{"unicode escape in a key", `{"` + esc(0x0041) + `":1}`, `{"A":n1}`},
		{"surrogate pair", `["` + esc(0xD83D) + esc(0xDE00) + `"]`, `[s"😀"]`},
		{"surrogate pair in uppercase hex", `["` + escUpper(0xD83D) + escUpper(0xDE00) + `"]`, `[s"😀"]`},
		{"surrogate pair among text", `["a` + esc(0xD83D) + esc(0xDE00) + `b"]`, `[s"a😀b"]`},
		{"two surrogate pairs", `["` + esc(0xD83D) + esc(0xDE00) + esc(0xD83D) + esc(0xDE01) + `"]`, `[s"😀😁"]`},
		{"escape in key", `{"a\"b\n":1}`, `{"a\"b\n":n1}`},
		{"replacement character escaped", `["` + esc(0xFFFD) + `"]`, `[s"�"]`},
		{"replacement character raw", "[\"�\"]", `[s"�"]`},
		{"escaped backslash before u", `["\\ud800"]`, `[s"\\ud800"]`},
		{"non-ascii raw", `{"鍵":"値"}`, `{"鍵":s"値"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dump(parseStrict(t, tt.src)); got != tt.want {
				t.Errorf("Parse(%q) = %s, want %s", tt.src, got, tt.want)
			}
		})
	}
}

// TestParseKeepsNumberText covers the reason numbers are held as text: a
// float64 would lose the precision, the notation and the trailing zeros that
// a document pino never edits has to be written back with.
func TestParseKeepsNumberText(t *testing.T) {
	raws := []string{
		"0", "-0", "1", "-1", "1.50", "0.1", "1e10", "1E+10", "1.0e-7",
		"12345678901234567890", "123456789012345678901234567890.5",
	}

	for _, raw := range raws {
		t.Run(raw, func(t *testing.T) {
			node := parseStrict(t, "["+raw+"]")

			arr, ok := node.(*domain.Array)
			if !ok {
				t.Fatalf("parsed %T, want *domain.Array", node)
			}

			num, ok := arr.At(0).(*domain.Number)
			if !ok {
				t.Fatalf("element is %T, want *domain.Number", arr.At(0))
			}

			if num.Raw() != raw {
				t.Errorf("Raw() = %q, want %q", num.Raw(), raw)
			}
		})
	}
}

// TestParseCopiesSource guards the tree against the buffer it was parsed from.
// The library's literals alias that buffer, so a node holding one directly
// would change when the caller reused the bytes.
func TestParseCopiesSource(t *testing.T) {
	src := []byte(`{"key":"value","n":1234}`)

	node, err := New().Parse(src, domain.StrictJSON)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	before := dump(node)

	for i := range src {
		src[i] = 'x'
	}

	if after := dump(node); after != before {
		t.Errorf("tree changed with the source: %s, was %s", after, before)
	}
}

func TestParseSyntaxError(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		line, column int
	}{
		{"empty", ``, 1, 1},
		{"whitespace only", "  \n  ", 2, 3},
		{"unclosed object", `{`, 1, 2},
		{"unclosed array", `[1`, 1, 3},
		{"missing colon", `{"a" 1}`, 1, 6},
		{"missing comma", `[1 2]`, 1, 4},
		{"unquoted key", `{a:1}`, 1, 2},
		{"single quotes", `'a'`, 1, 1},
		{"garbage after value", `1 2`, 1, 3},
		{"bad literal", `[tru]`, 1, 2},
		{"second line", "{\n  \"a\": :\n}", 2, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := syntaxErrorFor(t, tt.src, domain.StrictJSON)
			wantPosition(t, err, tt.line, tt.column)

			if err.Msg == "" {
				t.Error("Msg is empty")
			}

			if strings.Contains(err.Msg, "hujson:") {
				t.Errorf("Msg still names the library: %q", err.Msg)
			}
		})
	}
}

// TestParseDuplicateKey covers pino's rule that every path in a document is
// unique, and that the offending key is pointed at rather than the object.
func TestParseDuplicateKey(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		key          string
		line, column int
	}{
		{"adjacent", `{"a":1,"a":2}`, "a", 1, 8},
		{"apart", `{"a":1,"b":2,"a":3}`, "a", 1, 14},
		{"nested", `{"o":{"b":1,"b":2}}`, "b", 1, 13},
		{"in array", `[{"a":1,"a":2}]`, "a", 1, 9},
		{"a key escaped into the same text", `{"` + esc(0x61) + `":1,"a":2}`, "a", 1, 13},
		{"across lines", "{\n  \"a\": 1,\n  \"a\": 2\n}", "a", 3, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := syntaxErrorFor(t, tt.src, domain.StrictJSON)
			wantPosition(t, err, tt.line, tt.column)

			var dup *domain.DuplicateKeyError
			if !errors.As(err, &dup) {
				t.Fatalf("error does not unwrap to *domain.DuplicateKeyError: %v", err)
			}

			if dup.Key != tt.key {
				t.Errorf("Key = %q, want %q", dup.Key, tt.key)
			}
		})
	}
}

// TestParseInvalidUTF8 covers text the document cannot be written back with.
// The library unescapes through encoding/json, which substitutes U+FFFD rather
// than failing, so these would otherwise enter the tree silently changed.
func TestParseInvalidUTF8(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		line, column int
	}{
		{"lone continuation byte", "[\"x\x80y\"]", 1, 4},
		{"truncated sequence", "[\"\xe3\x81\"]", 1, 3},
		{"overlong encoding", "[\"\xc0\xaf\"]", 1, 3},
		{"surrogate half as bytes", "[\"\xed\xa0\x80\"]", 1, 3},
		{"in a key", "{\"\xff\":1}", 1, 3},
		{"nested", "{\"a\":{\"b\":[\"\xff\"]}}", 1, 13},
		{"second line", "[\n  \"\xff\"\n]", 2, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := syntaxErrorFor(t, tt.src, domain.StrictJSON)
			wantPosition(t, err, tt.line, tt.column)

			if !strings.Contains(err.Msg, "UTF-8") {
				t.Errorf("Msg = %q, want it to mention UTF-8", err.Msg)
			}
		})
	}
}

// TestParseUnpairedSurrogateEscape covers the case the byte-level UTF-8 check
// cannot see: the escape is ASCII, but encoding/json still decodes it to
// U+FFFD, so the document would be rewritten on save.
func TestParseUnpairedSurrogateEscape(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		line, column int
	}{
		{"high surrogate alone", `["\ud800"]`, 1, 3},
		{"low surrogate alone", `["\udc00"]`, 1, 3},
		{"high surrogate followed by text", `["\ud800x"]`, 1, 3},
		{"high surrogate followed by another high", `["\ud800\ud800"]`, 1, 3},
		{"high surrogate followed by a non-surrogate escape", `["\ud800A"]`, 1, 3},
		{"after other text", `["ab\udc00"]`, 1, 5},
		{"in a key", `{"\ud800":1}`, 1, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := syntaxErrorFor(t, tt.src, domain.StrictJSON)
			wantPosition(t, err, tt.line, tt.column)

			if !strings.Contains(err.Msg, "surrogate") {
				t.Errorf("Msg = %q, want it to mention a surrogate", err.Msg)
			}
		})
	}
}

func TestLoneSurrogateIndex(t *testing.T) {
	tests := []struct {
		name string
		lit  string
		want int
	}{
		{"no escape", `"abc"`, -1},
		{"ordinary escape", `"a\nb"`, -1},
		{"non-surrogate unicode escape", `"` + esc(0x0041) + `"`, -1},
		{"escape just below the surrogate range", `"` + esc(0xD7FF) + `"`, -1},
		{"escape just above the surrogate range", `"` + esc(0xE000) + `"`, -1},
		{"pair", `"` + esc(0xD83D) + esc(0xDE00) + `"`, -1},
		{"pair in uppercase hex", `"` + escUpper(0xD83D) + escUpper(0xDE00) + `"`, -1},
		{"pair among text", `"a` + esc(0xD83D) + esc(0xDE00) + `b"`, -1},
		{"two pairs", `"` + esc(0xD83D) + esc(0xDE00) + esc(0xD83D) + esc(0xDE01) + `"`, -1},
		{"escaped backslash then a surrogate spelling", `"\\ud800"`, -1},
		{"high alone", `"` + esc(0xD800) + `"`, 1},
		{"low alone", `"` + esc(0xDC00) + `"`, 1},
		{"low before high", `"` + esc(0xDC00) + esc(0xD800) + `"`, 1},
		{"high followed by a non-surrogate escape", `"` + esc(0xD800) + esc(0x0041) + `"`, 1},
		{"high after a pair", `"` + esc(0xD83D) + esc(0xDE00) + esc(0xD800) + `"`, 13},
		{"high then short input", `"` + esc(0xD800) + `\u`, 1},
		{"escape that is not hexadecimal", `"\uZZZZ"`, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := loneSurrogateIndex([]byte(tt.lit)); got != tt.want {
				t.Errorf("loneSurrogateIndex(%q) = %d, want %d", tt.lit, got, tt.want)
			}
		})
	}
}

func TestInvalidUTF8Index(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"ascii", "abc", -1},
		{"multi-byte", "鍵é😀", -1},
		{"replacement character", "a�b", -1},
		{"empty", "", -1},
		{"lone continuation", "a\x80", 1},
		{"truncated", "\xe3\x81", 0},
		{"overlong", "\xc0\xaf", 0},
		{"surrogate bytes", "\xed\xa0\x80", 0},
		{"invalid after valid", "鍵\xff", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := invalidUTF8Index([]byte(tt.text)); got != tt.want {
				t.Errorf("invalidUTF8Index(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}
