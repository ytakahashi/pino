package documentview

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// cursorRow is everything the layers above read from a row the cursor can
// land on, short of the text drawn in it.
//
// The pointer stands in for the Path because a Path holds a slice and so is
// not comparable; within a document the two identify a node equally well.
type cursorRow struct {
	Pointer   string
	Kind      LineKind
	Depth     int
	Collapsed bool
}

func (r cursorRow) String() string {
	return fmt.Sprintf("%-7s %d %-24s %t", r.Kind, r.Depth, r.Pointer, r.Collapsed)
}

// cursorRows is every selectable row, in order. The two views may place comment
// rows differently because tree view has no closing row for a container.
func cursorRows(lines []Line) []cursorRow {
	rows := make([]cursorRow, 0, len(lines))

	for _, l := range lines {
		if !l.Kind.Selectable() {
			continue
		}

		pointer := l.Path.String()
		if pointer == "" {
			pointer = "/"
		}

		rows = append(rows, cursorRow{
			Pointer:   pointer,
			Kind:      l.Kind,
			Depth:     l.Depth,
			Collapsed: l.Collapsed,
		})
	}

	return rows
}

func formatRows(rows []cursorRow) string {
	var b strings.Builder

	for i, r := range rows {
		fmt.Fprintf(&b, "%2d  %s\n", i, r)
	}

	return b.String()
}

// maxFoldable is how many foldable nodes a document of the corpus may have.
//
// foldings enumerates every subset, so the cost doubles with each one. The
// limit is a prompt rather than a rule: a document large enough to trip it is
// saying something a smaller one could say, since what is being checked here
// is a property of every node rather than of the document as a whole.
const maxFoldable = 8

// foldings is every state the folded set of a document can be in.
//
// Every subset is used rather than a few chosen ones, because a set chosen to
// exercise the property is a set chosen by whoever believes it already holds.
// The document's own options are included as well: they carry entries that
// fold nothing, which no subset of the foldable nodes would produce, and both
// views have to ignore them alike.
func foldings(t *testing.T, doc document) []map[string]struct{} {
	t.Helper()

	foldable := foldablePointers(doc.root)
	if len(foldable) > maxFoldable {
		t.Fatalf("the document has %d foldable nodes, more than the %d this enumerates; "+
			"split it, or say here why the whole power set is still wanted",
			len(foldable), maxFoldable)
	}

	sets := make([]map[string]struct{}, 0, 1<<len(foldable)+1)

	for mask := range 1 << len(foldable) {
		set := make(map[string]struct{}, len(foldable))

		for i, p := range foldable {
			if mask&(1<<i) != 0 {
				set[p] = struct{}{}
			}
		}

		sets = append(sets, set)
	}

	return append(sets, doc.opt.Collapsed)
}

// foldablePointers is every node of a document that folding can reach.
//
// A fully expanded JSON view names them one open row each, which is the same
// answer either renderer would give: an empty container is drawn on one row in
// both, and so cannot be opened or closed.
func foldablePointers(root domain.Node) []string {
	var pointers []string

	for _, l := range NewJSONRenderer().Render(root, Options{}) {
		if l.Kind == LineOpen {
			pointers = append(pointers, l.Path.String())
		}
	}

	return pointers
}

func equalRows(a, b []cursorRow) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// ownedComments groups rendered comment spans by the selectable row whose
// path owns them. The physical row may differ between views: JSON can append
// a container comment to its closing row, while tree view has no such row.
type ownedComments struct {
	Pointer  string
	Comments []string
}

func commentsByCursorRow(lines []Line) []ownedComments {
	var comments []ownedComments
	owner := make(map[string]int)

	for _, line := range lines {
		if !line.Kind.Selectable() {
			continue
		}

		pointer := line.Path.String()
		owner[pointer] = len(comments)
		comments = append(comments, ownedComments{Pointer: pointer})
	}

	for _, line := range lines {
		pointer := line.Path.String()
		i, ok := owner[pointer]
		if !ok {
			// There should be no rendered comment without a selectable owner.
			// Keeping one in the result makes that invariant observable instead
			// of silently dropping a bad Path from this comparison.
			i = len(comments)
			owner[pointer] = i
			comments = append(comments, ownedComments{Pointer: pointer})
		}

		for _, span := range line.Spans {
			if span.Role == RoleComment {
				comments[i].Comments = append(comments[i].Comments, span.Text)
			}
		}
	}

	return comments
}

func equalComments(a, b []ownedComments) bool {
	return slices.EqualFunc(a, b, func(a, b ownedComments) bool {
		return a.Pointer == b.Pointer && slices.Equal(a.Comments, b.Comments)
	})
}

func formatComments(comments []ownedComments) string {
	var b strings.Builder

	for i, entry := range comments {
		pointer := entry.Pointer
		if pointer == "" {
			pointer = "/"
		}
		fmt.Fprintf(&b, "%2d  %-24s %q\n", i, pointer, entry.Comments)
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
