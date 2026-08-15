package application

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/application/testdocs"
	"github.com/ytakahashi/pino/internal/domain"
)

// documents is the corpus the session is checked against.
//
// It is the corpus the renderers draw as well, which is what lets a document
// added for one of them be replayed by the properties here: the same trees are
// switched between views, edited and undone. Only the trees are wanted, since
// how a document is drawn is what the renderer tests read the corpus for.
func documents(t *testing.T) map[string]domain.Node {
	t.Helper()

	corpus := testdocs.Documents()
	roots := make(map[string]domain.Node, len(corpus))

	for name, d := range corpus {
		roots[name] = d.Root
	}

	return roots
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

	return testdocs.Sample()
}

// path builds a Path the way the renderer walks a tree.
func path(segs ...domain.Segment) domain.Path {
	p := domain.Path{}
	for _, s := range segs {
		p = p.Child(s)
	}

	return p
}

// folded is a set of folded nodes, keyed the way the drawing options want it.
func folded(pointers ...string) map[string]struct{} {
	return testdocs.Folded(pointers...)
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
// It is what a failure compares two sessions by: the rows say what each of
// them holds, roles included, and nothing about how either is looking at it.
func dumpLines(lines []documentview.Line) string {
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
		// message readable; pointers that need none come out unchanged.
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

// describe names a folded set in a failure message. The pointers are sorted so
// that the same set reads the same way twice.
func describe(set map[string]struct{}) string {
	if len(set) == 0 {
		return "nothing"
	}

	pointers := make([]string, 0, len(set))
	for p := range set {
		if p == "" {
			p = "/"
		}

		pointers = append(pointers, p)
	}

	slices.Sort(pointers)

	return strings.Join(pointers, " ")
}
