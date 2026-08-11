package domain

import "testing"

// A node carries its own comments only for an array element and for the root:
// in an object they belong to the member. Nothing can attach one yet, and no
// constructor takes one, so these tests are inside the package — exporting a
// way to build such a tree would be exporting an API for the tests alone.
//
// They matter now because path copying is where comments get dropped, and the
// day a JSONC document can hold one is not the day to find out.

func note(text string) Trivia {
	return NewTrivia([]Comment{{Text: text}}, nil)
}

// commented is an array whose elements, and the array itself, carry comments,
// held inside an object so that the array is reached through a rebuild.
//
//	{ "features": [ "search", "history" ] }
func commented(t *testing.T) (root *Object, array *Array) {
	t.Helper()

	first, err := NewString("search")
	if err != nil {
		t.Fatalf("NewString: %v", err)
	}

	first.trivia = note(" the first one")

	second, err := NewString("history")
	if err != nil {
		t.Fatalf("NewString: %v", err)
	}

	array = NewArray([]Node{first, second})
	array.trivia = note(" what the tool can do")

	root, err = NewObject([]Member{{Key: "features", Value: array}})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	root.trivia = note(" the whole document")

	return root, array
}

func TestRebuildingAContainerKeepsTheCommentsAroundIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		edit func(root *Object) (EditResult, error)
	}{
		{
			name: "replacing an element",
			edit: func(root *Object) (EditResult, error) {
				return SetValue(root, pathOf(KeySegment("features"), IndexSegment(1)), NewNumber("1"))
			},
		},
		{
			name: "adding an element",
			edit: func(root *Object) (EditResult, error) {
				return Insert(root, pathOf(KeySegment("features")), 0, Member{Value: NewNull()})
			},
		},
		{
			name: "removing an element",
			edit: func(root *Object) (EditResult, error) {
				return Delete(root, pathOf(KeySegment("features"), IndexSegment(0)))
			},
		},
		{
			name: "renaming the member holding it",
			edit: func(root *Object) (EditResult, error) {
				return Rename(root, pathOf(KeySegment("features")), "abilities")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root, _ := commented(t)

			res, err := tt.edit(root)
			if err != nil {
				t.Fatalf("edit: %v", err)
			}

			// Both the container that was rebuilt and the root above it.
			if res.Root.Trivia().IsEmpty() {
				t.Error("the comments on the root were dropped")
			}

			for _, m := range res.Root.(*Object).All() {
				if m.Value.Trivia().IsEmpty() {
					t.Errorf("the comments on the array under %q were dropped", m.Key)
				}
			}
		})
	}
}

func TestReplacingAValueKeepsTheCommentsAtThatPlace(t *testing.T) {
	t.Parallel()

	// The comments sit at a position in the document, not on the value that
	// happens to be there, so typing a new value over the old one leaves them.
	element := pathOf(KeySegment("features"), IndexSegment(0))

	tests := []struct {
		name string
		edit func(root *Object) (EditResult, error)
	}{
		{
			name: "a new value",
			edit: func(root *Object) (EditResult, error) {
				return SetValue(root, element, NewNumber("1"))
			},
		},
		{
			name: "a new type",
			edit: func(root *Object) (EditResult, error) {
				return ChangeType(root, element, KindNumber)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root, _ := commented(t)

			res, err := tt.edit(root)
			if err != nil {
				t.Fatalf("edit: %v", err)
			}

			n, ok := Resolve(res.Root, element)
			if !ok {
				t.Fatal("the element is gone")
			}

			if n.Trivia().IsEmpty() {
				t.Error("the comments on the element were dropped")
			}
		})
	}
}

func TestChangingToTheTypeItHasKeepsTheTreeItself(t *testing.T) {
	t.Parallel()

	// withTrivia builds a new node when it has something to carry, so a change
	// of type that is not a change has to stop before reaching it.
	root, _ := commented(t)
	element := pathOf(KeySegment("features"), IndexSegment(0))

	res, err := ChangeType(root, element, KindString)
	if err != nil {
		t.Fatalf("ChangeType: %v", err)
	}

	if res.Root != Node(root) {
		t.Error("a new tree came back from a change to the type already there")
	}
}

// pathOf builds a Path from segments. The exported tests have their own; this
// one is here because an internal test cannot see it.
func pathOf(segs ...Segment) Path {
	p := Path{}
	for _, s := range segs {
		p = p.Child(s)
	}

	return p
}
