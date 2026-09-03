package documentview

import (
	"slices"
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

func TestRenderersHideCommentsInsideAFoldedContainer(t *testing.T) {
	t.Parallel()

	doc := documents(t)["comments"]
	opt := Options{Collapsed: map[string]struct{}{"/empty-object": {}}}

	for name, renderer := range map[string]Renderer{
		"JSON": NewJSONRenderer(),
		"tree": NewTreeRenderer(),
	} {
		t.Run(name, func(t *testing.T) {
			lines := renderer.Render(doc.root, opt)
			for _, line := range lines {
				if strings.Contains(line.Text(), "banner") || strings.Contains(line.Text(), "kept as written") {
					t.Errorf("folded comment is still visible on %q", line.Text())
				}
			}

			if !slices.ContainsFunc(lines, func(line Line) bool {
				return line.Path.String() == "/empty-object" && line.Collapsed
			}) {
				t.Error("the comment-only container is not drawn folded")
			}
		})
	}
}

// The tree goldens sit beside the JSON goldens of the same documents, so that
// the two can be read side by side: the rows the cursor can land on are the
// same rows, in the same order, which is what TestViewsAgreeOnRows fixes.
func TestTreeRenderDrawsTheWholeDocument(t *testing.T) {
	t.Parallel()

	for name, doc := range documents(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			checkGolden(t, "tree-"+name, dumpLines(NewTreeRenderer().Render(doc.root, doc.opt)))
		})
	}
}

func TestTreeRenderReturnsNoLinesWithoutADocument(t *testing.T) {
	t.Parallel()

	if lines := NewTreeRenderer().Render(nil, Options{}); lines != nil {
		t.Errorf("Render(nil) = %v, want nil", lines)
	}
}

func TestTreeRenderPromotesAnExpandedMemberTrailingComment(t *testing.T) {
	t.Parallel()

	lines := NewTreeRenderer().Render(documents(t)["comments"].root, Options{})
	for _, line := range lines {
		if !strings.Contains(line.Text(), "nested complete") {
			continue
		}

		if line.Kind != LineComment || line.Path.String() != "/nested" || line.Depth != 1 {
			t.Errorf("trailing container comment = (%s, %q, depth %d), want (comment, %q, depth 1)",
				line.Kind, line.Path.String(), line.Depth, "/nested")
		}
		return
	}

	t.Error("trailing container comment was not rendered")
}

func TestTreeRenderPromotesAnExpandedElementTrailingComment(t *testing.T) {
	t.Parallel()

	object, err := domain.NewObject([]domain.Member{{Key: "leaf", Value: domain.NewNull()}})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}
	comment := commentForRenderTest(t, " element complete ", true, false)
	commented := domain.WithTrivia(object, domain.NewTrivia(nil, []domain.Comment{comment}, nil))

	lines := NewTreeRenderer().Render(domain.NewArray([]domain.Node{commented}), Options{})
	for _, line := range lines {
		if !strings.Contains(line.Text(), "element complete") {
			continue
		}

		if line.Kind != LineComment || line.Path.String() != "/0" || line.Depth != 1 {
			t.Errorf("trailing container comment = (%s, %q, depth %d), want (comment, %q, depth 1)",
				line.Kind, line.Path.String(), line.Depth, "/0")
		}
		return
	}

	t.Error("trailing container comment was not rendered")
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
// Comment and closing rows are not compared: the views use different
// structures for those, while cursor movement depends only on selectable
// rows. A renderer that grew or dropped a selectable row, or folded on a rule
// slightly its own, fails here rather than in the view switch it would break.
func TestViewsAgreeOnRows(t *testing.T) {
	t.Parallel()

	for name, doc := range documents(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for _, set := range foldings(t, doc) {
				opt := Options{Collapsed: set, MaxStrLen: doc.opt.MaxStrLen}

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

// The renderers may put a comment on different physical rows, but changing
// view must not change which node owns it, its text, or its order among the
// other comments owned by that node.
func TestViewsAgreeOnCommentsByOwner(t *testing.T) {
	t.Parallel()

	for name, doc := range documents(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for _, set := range foldings(t, doc) {
				opt := Options{Collapsed: set, MaxStrLen: doc.opt.MaxStrLen}

				jsonComments := commentsByCursorRow(NewJSONRenderer().Render(doc.root, opt))
				treeComments := commentsByCursorRow(NewTreeRenderer().Render(doc.root, opt))

				if !equalComments(jsonComments, treeComments) {
					t.Errorf("the views assign comments differently with %s folded\n json view:\n%s\n tree view:\n%s",
						describe(set), formatComments(jsonComments), formatComments(treeComments))
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
				opt := Options{Collapsed: set, MaxStrLen: doc.opt.MaxStrLen}

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
				opt := Options{Collapsed: set, MaxStrLen: doc.opt.MaxStrLen}
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

					case LineSingle, LineComment:
					}
				}

				if len(open) != 0 {
					t.Errorf("%d rows left open, with %s folded", len(open), describe(set))
				}
			}
		})
	}
}

func TestTreeNameQuotesOnlyUnsafeLabels(t *testing.T) {
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

func TestBadgeShowsTheContainerAndChildCount(t *testing.T) {
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
