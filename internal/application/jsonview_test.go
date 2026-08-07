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
// only what the layer above is given.
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

		fmt.Fprintf(&b, "%2d  %-6s  %d  %-20s  %s\n",
			i, l.Kind, l.Depth, pointer, strings.Join(spans, " "))
	}

	return b.String()
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

	tests := map[string]domain.Node{
		// Containers within containers, down to an array of strings.
		"nested": object(t,
			member("server", object(t,
				member("host", text(t, "localhost")),
				member("port", domain.NewNumber("8080")),
				member("features", domain.NewArray([]domain.Node{
					text(t, "search"),
					text(t, "history"),
				})),
			)),
		),

		// Every kind that occupies a single row, so that each Role appears.
		"scalars": object(t,
			member("str", text(t, "text")),
			member("num", domain.NewNumber("-12.5e3")),
			member("yes", domain.NewBool(true)),
			member("no", domain.NewBool(false)),
			member("nothing", domain.NewNull()),
		),

		// Empty containers, including one that is not the last member and so
		// has to carry a comma.
		"empty": object(t,
			member("obj", object(t)),
			member("arr", domain.NewArray(nil)),
			member("outer", object(t,
				member("inner", object(t)),
			)),
			member("last", domain.NewNumber("1")),
		),

		// An array at the root, holding objects and an array.
		"array-of-objects": domain.NewArray([]domain.Node{
			object(t,
				member("id", domain.NewNumber("1")),
				member("tags", domain.NewArray([]domain.Node{text(t, "a")})),
			),
			object(t, member("id", domain.NewNumber("2"))),
		}),

		// A document that is a single value.
		"root-scalar": text(t, "just a string"),

		// Text that has to be escaped, in the row and in the pointer. The
		// control characters matter twice over: unescaped, they would reach
		// the terminal.
		"escapes": object(t,
			member("a/b", text(t, `say "hi"`)),
			member("c~d", text(t, "tab\there")),
			member("ctrl", text(t, "nul:\x00 esc:\x1b")),
		),
	}

	for name, root := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := dumpLines(NewJSONRenderer().Render(root, RenderOptions{}))
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
