package application

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// The tree goldens sit beside the JSON goldens of the same documents, so that
// the two can be read side by side: the rows the cursor can land on are the
// same rows, in the same order, which is what TestViewsAgreeOnRows fixes.
func TestTreeRender(t *testing.T) {
	t.Parallel()

	for name, doc := range documents(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			checkGolden(t, "tree-"+name, dumpLines(NewTreeRenderer().Render(doc.root, doc.opt)))
		})
	}
}

func TestTreeRenderWithoutDocument(t *testing.T) {
	t.Parallel()

	if lines := NewTreeRenderer().Render(nil, RenderOptions{}); lines != nil {
		t.Errorf("Render(nil) = %v, want nil", lines)
	}
}

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
	return fmt.Sprintf("%-6s %d %-24s %t", r.Kind, r.Depth, r.Pointer, r.Collapsed)
}

// cursorRows is the rows a view offers the cursor, in order.
//
// A close row is left out because it is the one row the cursor never lands on,
// and the one row the tree view has no use for.
func cursorRows(lines []Line) []cursorRow {
	rows := make([]cursorRow, 0, len(lines))

	for _, l := range lines {
		if l.Kind == LineClose {
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

// TestViewsAgreeOnRows fixes the property the whole two-view design rests on.
//
// Both renderers decide what to draw from the document and the folded set
// alone, and walk the tree in the same order, so the rows the cursor can land
// on are the same nodes in the same positions whichever renderer drew them.
// Everything Tab has to do follows from that: the cursor is held as a Path, so
// the row carrying it exists in the view being switched to, and j, k, gg, G,
// zR and zM walk the same nodes in either view.
//
// More than the pointer is compared. Kind and Collapsed are what h and l read
// to decide whether a row can be opened and whether it already is, so the two
// views mean the same thing by those keys only if they agree here too, and
// Depth is what the indentation of every row is worked out from.
//
// A renderer that grew a row of its own, or dropped one, or folded on a rule
// slightly its own, fails here rather than in the view switch it would break.
func TestViewsAgreeOnRows(t *testing.T) {
	t.Parallel()

	for name, doc := range documents(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for _, set := range foldings(t, doc) {
				opt := RenderOptions{Collapsed: set, MaxStrLen: doc.opt.MaxStrLen}

				jsonRows := cursorRows(NewJSONRenderer().Render(doc.root, opt))
				treeRows := cursorRows(NewTreeRenderer().Render(doc.root, opt))

				if !equalRows(jsonRows, treeRows) {
					t.Errorf("the views disagree with %s folded\n json view:\n%s\n tree view:\n%s",
						describe(set), formatRows(jsonRows), formatRows(treeRows))
				}
			}
		})
	}
}

// The tree view has no closing rows: a container ends where the depth of the
// rows drops back, so a row holding nothing but a bracket would be a blank
// line on screen. Nothing above depends on their being there, which is why the
// pairing of an open row with a close row is a property of the JSON view
// rather than of the Line model.
func TestTreeRenderHasNoCloseRows(t *testing.T) {
	t.Parallel()

	for name, doc := range documents(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for _, set := range foldings(t, doc) {
				opt := RenderOptions{Collapsed: set, MaxStrLen: doc.opt.MaxStrLen}

				for i, l := range NewTreeRenderer().Render(doc.root, opt) {
					if l.Kind == LineClose {
						t.Errorf("row %d is a close row (%s), with %s folded",
							i, l.Path.String(), describe(set))
					}
				}
			}
		})
	}
}

// The other half of the same statement, which does hold: a close row always
// closes the nearest open row still waiting for one, and carries its path.
// Folding, deleting a subtree and caching one all treat the two as a pair, so
// a JSON view that opened without closing, or closed something other than what
// it opened, would break all three.
func TestJSONRenderClosesWhatItOpens(t *testing.T) {
	t.Parallel()

	for name, doc := range documents(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for _, set := range foldings(t, doc) {
				opt := RenderOptions{Collapsed: set, MaxStrLen: doc.opt.MaxStrLen}
				lines := NewJSONRenderer().Render(doc.root, opt)

				var open []domain.Path

				for i, l := range lines {
					switch l.Kind {
					case LineOpen:
						open = append(open, l.Path)

					case LineClose:
						if len(open) == 0 {
							t.Fatalf("row %d closes %q with nothing open, with %s folded",
								i, l.Path.String(), describe(set))
						}

						last := open[len(open)-1]
						open = open[:len(open)-1]

						if !last.Equal(l.Path) {
							t.Errorf("row %d closes %q, want %q, with %s folded",
								i, l.Path.String(), last.String(), describe(set))
						}

					case LineSingle:
					}
				}

				if len(open) != 0 {
					t.Errorf("%d rows left open, with %s folded", len(open), describe(set))
				}
			}
		})
	}
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

	for _, l := range NewJSONRenderer().Render(root, RenderOptions{}) {
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

func TestTreeName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		label string
		want  string
	}{
		// Names that read as themselves keep their quotes off. Spaces and
		// text outside ASCII are among them: neither can break a row, and
		// quoting them would put marks on most keys in a document written in
		// a language other than English.
		"plain":       {label: "host", want: "host"},
		"with space":  {label: "with space", want: "with space"},
		"non-ASCII":   {label: "設定", want: "設定"},
		"a solidus":   {label: "a/b", want: "a/b"},
		"a tilde":     {label: "c~d", want: "c~d"},
		"a digit":     {label: "0", want: "0"},
		"the root":    {label: rootLabel, want: "/"},
		"punctuation": {label: "{}", want: "{}"},

		// Names that do not. The control characters are the reason the rule
		// exists: drawn bare they would split the row or reach the terminal
		// as an escape sequence.
		"empty":       {label: "", want: `""`},
		"a quote":     {label: `say "hi"`, want: `"say \"hi\""`},
		"a backslash": {label: `back\slash`, want: `"back\\slash"`},
		"a tab":       {label: "tab\there", want: `"tab\there"`},
		"a newline":   {label: "nl\nhere", want: `"nl\nhere"`},
		"an escape":   {label: "esc\x1bhere", want: `"esc\u001bhere"`},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := treeName(tc.label)

			if got.Text != tc.want {
				t.Errorf("treeName(%q) = %s, want %s", tc.label, got.Text, tc.want)
			}

			if got.Role != RoleKey {
				t.Errorf("Role = %v, want %v; quoting does not make a name something else",
					got.Role, RoleKey)
			}
		})
	}
}

func TestBadge(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		left, right string
		n           int
		want        string
	}{
		"an empty object": {left: "{", right: "}", n: 0, want: " {}"},
		"an empty array":  {left: "[", right: "]", n: 0, want: " []"},
		"one member":      {left: "{", right: "}", n: 1, want: " {1}"},
		"three members":   {left: "{", right: "}", n: 3, want: " {3}"},
		"two elements":    {left: "[", right: "]", n: 2, want: " [2]"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := badge(tc.left, tc.right, tc.n)

			if got.Text != tc.want {
				t.Errorf("badge(%q, %q, %d) = %q, want %q", tc.left, tc.right, tc.n, got.Text, tc.want)
			}

			if got.Role != RolePunct {
				t.Errorf("Role = %v, want %v", got.Role, RolePunct)
			}
		})
	}
}
