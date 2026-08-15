package documentview

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/application/testdocs"
	"github.com/ytakahashi/pino/internal/domain"
)

var update = flag.Bool("update", false, "rewrite the golden files instead of comparing against them")

// document is one document of the corpus, together with the options it is
// drawn under.
type document struct {
	root domain.Node
	opt  Options // the zero value draws everything, in full
}

// documents is the corpus every renderer is tested against.
//
// It comes from testdocs rather than from this file because the session tests
// in the parent package are checked against the same trees: a document added
// for one view is then drawn by the other one too, and replayed by the edit
// and undo properties as well.
func documents(t *testing.T) map[string]document {
	t.Helper()

	corpus := testdocs.Documents()
	docs := make(map[string]document, len(corpus))

	for name, d := range corpus {
		docs[name] = document{
			root: d.Root,
			opt: Options{
				Collapsed: testdocs.Folded(d.Collapsed...),
				MaxStrLen: d.MaxStrLen,
			},
		}
	}

	return docs
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
		t.Fatalf("reading %s: %v (run go test ./internal/application/documentview -update to create it)", golden, err)
	}

	if got != string(want) {
		t.Errorf("rendered rows differ from %s\n got:\n%s\nwant:\n%s", golden, got, want)
	}
}
