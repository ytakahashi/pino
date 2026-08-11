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

// document is one document of the corpus, together with the options it is
// drawn under.
type document struct {
	root domain.Node
	opt  RenderOptions // the zero value draws everything, in full
}

// documents is the corpus every renderer is tested against.
//
// It is shared rather than written out per renderer so that a document added
// for one view is drawn by the other one too, and so that the property tying
// the views together is checked against a growing set of documents rather than
// a set chosen to satisfy it. A document belongs here when it says something
// about how a structure is drawn; one that exists to exercise a single helper
// belongs next to that helper's test.
func documents(t *testing.T) map[string]document {
	t.Helper()

	return map[string]document{
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

		// A document that is nothing but an empty container. It is the one
		// document whose root offers nothing to unfold.
		"empty-root": {root: object(t)},

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

		// Keys that decide how a view names a member. The JSON view quotes
		// every one of them; a view that does not has to tell the ones that
		// read as themselves from the ones that would reach the terminal or
		// leave the row unreadable.
		"keys": {root: object(t,
			member("", text(t, "no name at all")),
			member("plain", text(t, "a")),
			member("with space", text(t, "b")),
			member("設定", text(t, "c")),
			member(`say "hi"`, text(t, "d")),
			member(`back\slash`, text(t, "e")),
			member("tab\there", text(t, "f")),
			member("nl\nhere", text(t, "g")),
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
}

// sample is a document with a container holding a container, an array of
// scalars, and members on either side of them.
//
//	 0  open    /                {
//	 1  single  /name            "name": "pino",
//	 2  open    /server          "server": {
//	 3  single  /server/host       "host": "localhost",
//	 4  open    /server/ports      "ports": [
//	 5  single  /server/ports/0      8080,
//	 6  single  /server/ports/1      8443
//	 7  close   /server/ports      ],
//	 8  single  /server/tls        "tls": true
//	 9  close   /server          },
//	10  single  /debug           "debug": false
//	11  close   /                }
func sample(t *testing.T) domain.Node {
	t.Helper()

	return object(t,
		member("name", text(t, "pino")),
		member("server", object(t,
			member("host", text(t, "localhost")),
			member("ports", domain.NewArray([]domain.Node{
				domain.NewNumber("8080"),
				domain.NewNumber("8443"),
			})),
			member("tls", domain.NewBool(true)),
		)),
		member("debug", domain.NewBool(false)),
	)
}

// path builds a Path the way the renderer walks a tree.
func path(segs ...domain.Segment) domain.Path {
	p := domain.Path{}
	for _, s := range segs {
		p = p.Child(s)
	}

	return p
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

// dumpLines writes the rows in a form a reviewer can read.
//
// Roles are included because they are half of what the renderer decides, and
// styles are not, so the golden files survive a change of colours and show
// only what the layer above is given. Collapsed is there for the same reason:
// a folded row is a LineSingle whose spans happen to read "{…}", and without
// the flag a renderer that drew the braces but never set it would pass.
//
// The root is shown as "/", which is also the pointer of a member whose key is
// empty. The depth column tells the two apart, and spelling the root any other
// way would say less to a reader than it costs in noise on every other row.
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
		//
		// A key holding a tab or a newline reaches the pointer unescaped, and
		// would split this row or throw the columns out. Escaping keeps the
		// file readable; pointers that need none come out unchanged.
		pointer := domain.EscapeString(l.Path.String())
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

// checkGolden compares got against testdata/<name>.golden, or rewrites the
// file when -update is given.
func checkGolden(t *testing.T, name, got string) {
	t.Helper()

	golden := filepath.Join("testdata", name+".golden")

	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o600); err != nil {
			t.Fatalf("writing %s: %v", golden, err)
		}
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading %s: %v (run go test ./internal/application -update to create it)", golden, err)
	}

	if got != string(want) {
		t.Errorf("rendered rows differ from %s\n got:\n%s\nwant:\n%s", golden, got, want)
	}
}
