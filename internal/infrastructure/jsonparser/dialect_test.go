package jsonparser

import (
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

func TestParseRejectsComments(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		line, column int
	}{
		{"before the document", "// c\n{\"a\":1}", 1, 1},
		// The library needs a line comment to be closed by a newline, so the
		// fixtures that end in one carry it.
		{"after the document", "{\"a\":1} // c\n", 1, 9},
		{"after the opening brace", "{ // c\n\"a\":1}", 1, 3},
		{"before a key", "{\n  // c\n  \"a\": 1\n}", 2, 3},
		{"between key and value", `{"a": /* c */ 1}`, 1, 7},
		{"after a value", `{"a":1 /* c */}`, 1, 8},
		{"inside an empty object", `{ /* c */ }`, 1, 3},
		{"inside an empty array", `[ /* c */ ]`, 1, 3},
		{"between array elements", "[1, // c\n2]", 1, 5},
		{"after the last array element", `[1 /* c */]`, 1, 4},
		{"block comment spanning lines", "{\n  /* c\n     d */\n  \"a\": 1\n}", 2, 3},
		{"nested", `{"a":{"b":[/* c */1]}}`, 1, 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := syntaxErrorFor(t, tt.src, domain.StrictJSON)
			wantPosition(t, err, tt.line, tt.column)

			if !strings.Contains(err.Msg, "comment") {
				t.Errorf("Msg = %q, want it to mention a comment", err.Msg)
			}
		})
	}
}

func TestParseRejectsTrailingComma(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		line, column int
	}{
		{"array", `[1,]`, 1, 3},
		{"array with spacing", `[1 , ]`, 1, 4},
		{"array of one object", `[{"a":1},]`, 1, 9},
		{"object", `{"a":1,}`, 1, 7},
		{"object with spacing", `{"a":1 , }`, 1, 8},
		{"nested array", `{"a":[1,]}`, 1, 8},
		{"nested object", `[{"a":1,}]`, 1, 8},
		{"across lines", "[\n  1,\n]", 2, 4},
		{"outer of two", "[[1],]", 1, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := syntaxErrorFor(t, tt.src, domain.StrictJSON)
			wantPosition(t, err, tt.line, tt.column)

			if !strings.Contains(err.Msg, "trailing comma") {
				t.Errorf("Msg = %q, want it to mention a trailing comma", err.Msg)
			}
		})
	}
}

// TestParseAcceptsStandardJSON is the other half of the two tests above: the
// check has to leave ordinary documents alone, including the shapes that come
// closest to a trailing comma without being one.
func TestParseAcceptsStandardJSON(t *testing.T) {
	srcs := []string{
		`[]`,
		`{}`,
		`[ ]`,
		`{ }`,
		`[1]`,
		`{"a":1}`,
		`[1,2]`,
		`{"a":1,"b":2}`,
		`[ 1 , 2 ]`,
		`{ "a" : 1 , "b" : 2 }`,
		"{\n  \"a\": [\n    1,\n    2\n  ]\n}",
		`["a,b"]`,
		`["//not a comment"]`,
		`{"/*":"*/"}`,
		`[[],[[]]]`,
	}

	for _, src := range srcs {
		t.Run(src, func(t *testing.T) {
			parseStrict(t, src)
		})
	}
}

// TestParseDialectSelectsWhatIsRefused checks that the two extensions are
// refused independently, which is what supporting JSONC later turns on.
func TestParseDialectSelectsWhatIsRefused(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		dialect domain.Dialect
		want    string // the tree, or "" to expect an error
	}{
		{"comment allowed", "{\"a\":1} // c\n", domain.Dialect{AllowComments: true}, `{"a":n1}`},
		{"comment refused", "{\"a\":1} // c\n", domain.Dialect{AllowTrailingComma: true}, ""},
		{"trailing comma allowed", `[1,]`, domain.Dialect{AllowTrailingComma: true}, `[n1]`},
		{"trailing comma refused", `[1,]`, domain.Dialect{AllowComments: true}, ""},
		{"both allowed", "[\n  1, // c\n]", jwcc, `[n1]`},
		{"both refused", "[\n  1, // c\n]", domain.StrictJSON, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := New().Parse([]byte(tt.src), tt.dialect)

			if tt.want == "" {
				if err == nil {
					t.Fatalf("Parse(%q) = %s, want an error", tt.src, dump(node))
				}

				return
			}

			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.src, err)
			}

			if got := dump(node); got != tt.want {
				t.Errorf("Parse(%q) = %s, want %s", tt.src, got, tt.want)
			}
		})
	}
}

// TestParseStillChecksTheTreeUnderJWCC guards against the permissive dialect
// being read as permission to skip the rules that are pino's own rather than
// the format's.
func TestParseStillChecksTheTreeUnderJWCC(t *testing.T) {
	srcs := []string{
		`{"a":1,"a":2}`,
		"[\"\xff\"]",
		`["\ud800"]`,
	}

	for _, src := range srcs {
		t.Run(src, func(t *testing.T) {
			if err := syntaxErrorFor(t, src, jwcc); err.Line == 0 {
				t.Errorf("reported without a position: %v", err)
			}
		})
	}
}
