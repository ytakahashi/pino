package application

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

var update = flag.Bool("update", false, "rewrite the golden files instead of comparing against them")

// dumpLines writes the rows in a form a reviewer can read.
//
// Roles are included because they are half of what the renderer decides, and
// styles are not, so the golden files survive a change of colours and show
// only what the layer above is given. Collapsed is there for the same reason:
// a folded row is a LineSingle whose spans happen to read "{…}", and without
// the flag a renderer that drew the braces but never set it would pass.
func dumpLines(lines []Line) string {
	var b strings.Builder

	for i, l := range lines {
		// Brackets around the text keep a span such as ": " readable, whose
		// trailing space would otherwise be invisible in a diff.
		spans := make([]string, 0, len(l.Spans))
		for _, s := range l.Spans {
			spans = append(spans, s.Role.String()+"["+s.Text+"]")
		}

		// The root pointer is the empty string; "/" makes it visible here.
		pointer := l.Path.String()
		if pointer == "" {
			pointer = "/"
		}

		collapsed := ""
		if l.Collapsed {
			collapsed = "[C]"
		}

		fmt.Fprintf(&b, "%2d  %-6s  %d  %-20s  %-3s  %s\n",
			i, l.Kind, l.Depth, pointer, collapsed, strings.Join(spans, " "))
	}

	return b.String()
}

// folded is a set of folded nodes, keyed the way RenderOptions wants it.
func folded(pointers ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(pointers))
	for _, p := range pointers {
		set[p] = struct{}{}
	}

	return set
}

func member(key string, value domain.Node) domain.Member {
	return domain.Member{Key: key, Value: value}
}

func text(t *testing.T, v string) domain.Node {
	t.Helper()

	s, err := domain.NewString(v)
	if err != nil {
		t.Fatalf("NewString(%q): %v", v, err)
	}

	return s
}

func object(t *testing.T, members ...domain.Member) domain.Node {
	t.Helper()

	o, err := domain.NewObject(members)
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	return o
}

func TestJSONRender(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		root domain.Node
		opt  RenderOptions // the zero value draws everything, in full
	}{
		// Containers within containers, down to an array of strings.
		"nested": {root: object(t,
			member("server", object(t,
				member("host", text(t, "localhost")),
				member("port", domain.NewNumber("8080")),
				member("features", domain.NewArray([]domain.Node{
					text(t, "search"),
					text(t, "history"),
				})),
			)),
		)},

		// Every kind that occupies a single row, so that each Role appears.
		"scalars": {root: object(t,
			member("str", text(t, "text")),
			member("num", domain.NewNumber("-12.5e3")),
			member("yes", domain.NewBool(true)),
			member("no", domain.NewBool(false)),
			member("nothing", domain.NewNull()),
		)},

		// Empty containers, including one that is not the last member and so
		// has to carry a comma.
		"empty": {root: object(t,
			member("obj", object(t)),
			member("arr", domain.NewArray(nil)),
			member("outer", object(t,
				member("inner", object(t)),
			)),
			member("last", domain.NewNumber("1")),
		)},

		// An array at the root, holding objects and an array.
		"array-of-objects": {root: domain.NewArray([]domain.Node{
			object(t,
				member("id", domain.NewNumber("1")),
				member("tags", domain.NewArray([]domain.Node{text(t, "a")})),
			),
			object(t, member("id", domain.NewNumber("2"))),
		})},

		// A document that is a single value.
		"root-scalar": {root: text(t, "just a string")},

		// Text that has to be escaped, in the row and in the pointer. The
		// control characters matter twice over: unescaped, they would reach
		// the terminal.
		"escapes": {root: object(t,
			member("a/b", text(t, `say "hi"`)),
			member("c~d", text(t, "tab\there")),
			member("ctrl", text(t, "nul:\x00 esc:\x1b")),
		)},

		// Folded containers: one before its last sibling and one after it, so
		// that both sides of the separating comma are covered; one nested
		// inside a container still open; an array as well as an object; and a
		// key whose pointer has to be escaped to find it in the set.
		"collapsed": {
			root: object(t,
				member("server", object(t,
					member("cache", object(t, member("ttl", domain.NewNumber("60")))),
					member("host", text(t, "localhost")),
				)),
				member("features", domain.NewArray([]domain.Node{
					text(t, "search"),
					text(t, "history"),
				})),
				member("a/b", object(t, member("x", domain.NewNumber("1")))),
				member("opts", object(t, member("y", domain.NewNumber("2")))),
			),
			opt: RenderOptions{Collapsed: folded("/server/cache", "/features", "/a~1b", "/opts")},
		},

		// The whole document folded into one row. The root is keyed by the
		// empty pointer, which is the one entry easy to get wrong.
		"collapsed-root": {
			root: object(t,
				member("server", object(t, member("host", text(t, "localhost")))),
				member("port", domain.NewNumber("8080")),
			),
			opt: RenderOptions{Collapsed: folded("")},
		},

		// Entries in the set that cannot fold anything: empty containers,
		// which say as much unfolded, a scalar, and a node that is not there.
		"collapsed-ignored": {
			root: object(t,
				member("obj", object(t)),
				member("arr", domain.NewArray(nil)),
				member("num", domain.NewNumber("1")),
			),
			opt: RenderOptions{Collapsed: folded("/obj", "/arr", "/num", "/missing")},
		},

		// Values shortened at a limit small enough to read in the golden: one
		// under it, one exactly on it, one over; a key long enough to be cut
		// if keys were cut; text whose runes are wider than a byte; and a cut
		// that lands where an escape begins.
		"elided": {
			root: object(t,
				member("short", text(t, "abcd")),
				member("exact", text(t, "abcde")),
				member("over", text(t, "abcdef")),
				member("keeps-its-full-name", text(t, "x")),
				member("kanji", text(t, "設定ファイルです")),
				member("escaped", text(t, "ab\ncdef")),
				member("quotes", text(t, `ab"cdef`)),
			),
			opt: RenderOptions{MaxStrLen: 5},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := dumpLines(NewJSONRenderer().Render(tc.root, tc.opt))
			golden := filepath.Join("testdata", name+".golden")

			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o600); err != nil {
					t.Fatalf("writing %s: %v", golden, err)
				}
			}

			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("reading %s: %v (run go test -run TestJSONRender ./... -update to create it)", golden, err)
			}

			if got != string(want) {
				t.Errorf("rendered rows differ from %s\n got:\n%s\nwant:\n%s", golden, got, want)
			}
		})
	}
}

func TestJSONRenderWithoutDocument(t *testing.T) {
	t.Parallel()

	if lines := NewJSONRenderer().Render(nil, RenderOptions{}); lines != nil {
		t.Errorf("Render(nil) = %v, want nil", lines)
	}
}

func TestStringSpan(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value  string
		maxLen int
		want   string
	}{
		"shorter than the limit": {value: "abc", maxLen: 5, want: `"abc"`},
		"exactly the limit":      {value: "abcde", maxLen: 5, want: `"abcde"`},
		"one rune over":          {value: "abcdef", maxLen: 5, want: `"abcde…"`},
		"empty":                  {value: "", maxLen: 5, want: `""`},

		// No limit at all: the value is drawn however long it is, which is
		// what the golden files of the other tests rely on.
		"no limit":       {value: "abcdef", maxLen: 0, want: `"abcdef"`},
		"negative limit": {value: "abcdef", maxLen: -1, want: `"abcdef"`},

		// The limit counts runes, so a multi-byte character is one of them and
		// is never cut in half.
		"kanji under the limit": {value: "設定", maxLen: 5, want: `"設定"`},
		"kanji over the limit":  {value: "設定ファイル", maxLen: 3, want: `"設定フ…"`},
		"an emoji":              {value: "🌲🌲🌲", maxLen: 2, want: `"🌲🌲…"`},

		// A cut that lands next to text needing an escape. Escaping after the
		// cut is what keeps the sequence whole; cutting the escaped form could
		// leave "\u00" on screen.
		"cut before an escape": {value: "ab\ncd", maxLen: 2, want: `"ab…"`},
		"cut after an escape":  {value: "ab\ncd", maxLen: 3, want: `"ab\n…"`},
		"cut before a quote":   {value: `ab"cd`, maxLen: 2, want: `"ab…"`},
		"cut after a quote":    {value: `ab"cd`, maxLen: 3, want: `"ab\"…"`},
		"cut before a nul":     {value: "ab\x00cd", maxLen: 3, want: `"ab\u0000…"`},

		// A limit of one is still a limit, not a request for everything.
		"limit of one": {value: "abc", maxLen: 1, want: `"a…"`},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := stringSpan(tc.value, tc.maxLen)

			if got.Text != tc.want {
				t.Errorf("stringSpan(%q, %d) = %s, want %s", tc.value, tc.maxLen, got.Text, tc.want)
			}

			if got.Role != RoleStringValue {
				t.Errorf("Role = %v, want %v; shortening does not make a value something else",
					got.Role, RoleStringValue)
			}
		})
	}
}

// A value short enough to be drawn in full has to come out exactly as it would
// without a limit set, or the limit would be changing documents it does not
// shorten.
func TestStringSpanLeavesShortValuesAlone(t *testing.T) {
	t.Parallel()

	values := []string{"", "a", "abcde", "設定", `say "hi"`, "tab\there", "nul:\x00"}

	for _, v := range values {
		t.Run(v, func(t *testing.T) {
			t.Parallel()

			want := stringSpan(v, 0)
			if got := stringSpan(v, 64); got != want {
				t.Errorf("stringSpan(%q, 64) = %s, want %s", v, got.Text, want.Text)
			}
		})
	}
}

// The view state is what carries the limit to the renderer, so a document
// opened without anyone choosing one still has values shortened.
func TestNewViewStateShortensLongValues(t *testing.T) {
	t.Parallel()

	if got := NewViewState().RenderOptions().MaxStrLen; got <= 0 {
		t.Errorf("MaxStrLen = %d, want a limit; long values would be drawn in full", got)
	}
}

// TestJSONRenderSharesNoSpans guards the copying in spansOf: rows are built
// from a shared label, and appending a comma in place would write into the
// row that was built before it.
func TestJSONRenderSharesNoSpans(t *testing.T) {
	t.Parallel()

	root := object(t,
		member("a", domain.NewNumber("1")),
		member("b", domain.NewNumber("2")),
	)

	lines := NewJSONRenderer().Render(root, RenderOptions{})

	if got, want := lines[1].Text(), `"a": 1,`; got != want {
		t.Errorf("first member = %q, want %q", got, want)
	}

	if got, want := lines[2].Text(), `"b": 2`; got != want {
		t.Errorf("last member = %q, want %q", got, want)
	}
}
