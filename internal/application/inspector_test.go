package application

import (
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

func TestInspectorDescribesTheNodeAtTheCursor(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		pointer string
		want    InspectorInfo
	}{
		// The root is the one node that is a member of nothing, which is what
		// tells it apart from the member below whose key is empty.
		"the root": {
			pointer: "",
			want: InspectorInfo{
				Pointer: "", Type: "object", Container: true, Children: 10, Foldable: true, Naming: NamedNone,
			},
		},

		"a string": {
			pointer: "/str",
			want: InspectorInfo{
				Pointer: "/str", Type: "string",
				Value:  documentview.Span{Text: `"text"`, Role: documentview.RoleStringValue},
				Label:  "str",
				Naming: NamedKey,
			},
		},

		// A number is shown as the document spells it, exponent and all.
		"a number": {
			pointer: "/num",
			want: InspectorInfo{
				Pointer: "/num", Type: "number",
				Value:  documentview.Span{Text: "-12.5e3", Role: documentview.RoleNumberValue},
				Label:  "num",
				Naming: NamedKey,
			},
		},

		"a boolean": {
			pointer: "/yes",
			want: InspectorInfo{
				Pointer: "/yes", Type: "boolean",
				Value:  documentview.Span{Text: "true", Role: documentview.RoleBoolValue},
				Label:  "yes",
				Naming: NamedKey,
			},
		},

		"null": {
			pointer: "/nothing",
			want: InspectorInfo{
				Pointer: "/nothing", Type: "null",
				Value:  documentview.Span{Text: "null", Role: documentview.RoleNullValue},
				Label:  "nothing",
				Naming: NamedKey,
			},
		},

		// Containers report how many children they hold and no value: there is
		// no single line a container could be written on.
		"an object": {
			pointer: "/obj",
			want: InspectorInfo{
				Pointer: "/obj", Type: "object", Container: true, Children: 2, Foldable: true,
				Label: "obj", Naming: NamedKey,
			},
		},

		"an array": {
			pointer: "/arr",
			want: InspectorInfo{
				Pointer: "/arr", Type: "array", Container: true, Children: 2, Foldable: true,
				Label: "arr", Naming: NamedKey,
			},
		},

		// An empty container holds no children and has no value either, which
		// is what Container is there to tell apart from a scalar.
		"an empty object": {
			pointer: "/empty-obj",
			want: InspectorInfo{
				Pointer: "/empty-obj", Type: "object", Container: true, Children: 0,
				Label: "empty-obj", Naming: NamedKey,
			},
		},

		"an empty array": {
			pointer: "/empty-arr",
			want: InspectorInfo{
				Pointer: "/empty-arr", Type: "array", Container: true, Children: 0,
				Label: "empty-arr", Naming: NamedKey,
			},
		},

		// The pair a pointer cannot separate. "/arr/1" and "/digits/0" are
		// spelled alike and mean different things, and resolving them against
		// the document is what says which.
		"an element of an array": {
			pointer: "/arr/1",
			want: InspectorInfo{
				Pointer: "/arr/1", Type: "string",
				Value:  documentview.Span{Text: `"second"`, Role: documentview.RoleStringValue},
				Label:  "1",
				Naming: NamedIndex,
			},
		},

		"a member keyed with a digit": {
			pointer: "/digits/0",
			want: InspectorInfo{
				Pointer: "/digits/0", Type: "string",
				Value:  documentview.Span{Text: `"keyed, not indexed"`, Role: documentview.RoleStringValue},
				Label:  "0",
				Naming: NamedKey,
			},
		},

		// A member whose key is empty is still named: the label is empty, but
		// there is one, and the root above has none.
		"a member with an empty key": {
			pointer: "/",
			want: InspectorInfo{
				Pointer: "/", Type: "string",
				Value:  documentview.Span{Text: `"no name at all"`, Role: documentview.RoleStringValue},
				Label:  "",
				Naming: NamedKey,
			},
		},

		// A path that leads nowhere leaves the type empty, which is what says
		// nothing is selected.
		"a path that resolves to nothing": {
			pointer: "/missing",
			want:    InspectorInfo{},
		},

		"a path through a value": {
			pointer: "/str/deeper",
			want:    InspectorInfo{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := inspected(t, inspectorTree(t), tc.pointer); got != tc.want {
				t.Errorf("Inspector() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The inspector is where a value shortened on a row is read back, so it is the
// one place that never shortens.
func TestInspectorShowsLongValuesInFull(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", defaultMaxStrLen*2)
	root := object(t, member("long", text(t, long)))

	// The row does shorten it, or this would be checking nothing.
	rendered := documentview.NewJSONRenderer().Render(root, NewViewState().RenderOptions())
	if !strings.Contains(rendered[1].Text(), "…") {
		t.Fatalf("the row shows the value in full: %q", rendered[1].Text())
	}

	got := inspected(t, root, "/long")

	if want := `"` + long + `"`; got.Value.Text != want {
		t.Errorf("Value = %q, want the whole value", got.Value.Text)
	}
}

func TestInspectorSeparatesFoldableCommentsFromChildren(t *testing.T) {
	t.Parallel()

	comment, err := domain.NewComment(" inside", false, true)
	if err != nil {
		t.Fatalf("NewComment: %v", err)
	}
	empty := domain.WithTrivia(object(t), domain.NewTrivia(nil, nil, []domain.Comment{comment}))
	root := object(t, member("empty", empty))

	got := inspected(t, root, "/empty")
	if got.Children != 0 {
		t.Errorf("Children = %d, want 0", got.Children)
	}
	if !got.Foldable {
		t.Error("Foldable = false, want true")
	}
}

// Asking before a document is open has to answer rather than fail: the pane is
// drawn from the first frame onwards.
func TestInspectorIsEmptyWithoutADocument(t *testing.T) {
	t.Parallel()

	app := New(Deps{JSONView: documentview.NewJSONRenderer(), TreeView: documentview.NewTreeRenderer()}, Config{})

	if got := app.Inspector(); got != (InspectorInfo{}) {
		t.Errorf("Inspector() = %+v with no document open, want the zero value", got)
	}
}

// The inspector reads the tree, not the rows: producing them to describe one
// node would draw the whole document every time the pane is refreshed.
func TestInspectorDoesNotRender(t *testing.T) {
	t.Parallel()

	renderer := &fakeRenderer{lines: []documentview.Line{{Kind: documentview.LineOpen}}}

	app := New(Deps{
		Parser:   &fakeParser{root: testTree(t)},
		Files:    &fakeFileStore{data: map[string][]byte{"a.json": []byte(testSource)}},
		JSONView: renderer,
		TreeView: renderer,
	}, Config{})

	if err := app.Open("a.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	before := renderer.calls
	app.Inspector()

	if renderer.calls != before {
		t.Errorf("Inspector() rendered %d times, want none", renderer.calls-before)
	}
}

func TestNamingStringNamesEveryNodePosition(t *testing.T) {
	t.Parallel()

	tests := map[Naming]string{
		NamedNone:  "none",
		NamedKey:   "key",
		NamedIndex: "index",
		Naming(99): "unknown",
	}

	for naming, want := range tests {
		if got := naming.String(); got != want {
			t.Errorf("Naming(%d).String() = %q, want %q", naming, got, want)
		}
	}
}
