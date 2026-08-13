package domain

import (
	"errors"
	"slices"
	"testing"
)

func TestSetValueRebuildsOnlyThePathToTheNode(t *testing.T) {
	t.Parallel()

	d := newTree(t)
	p := at(t, "/server/port")

	res, err := SetValue(d.root, p, NewNumber("9090"))
	if err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	if res.Root == Node(d.root) {
		t.Fatal("the root came back unchanged after a value was replaced")
	}

	if !res.Cursor.Equal(p) {
		t.Errorf("Cursor = %q, want %q", res.Cursor, p)
	}

	if len(res.Renames) != 0 {
		t.Errorf("Renames = %v, want none: replacing a value moves nothing", pairs(res.Renames))
	}

	if got := nodeAt(t, res.Root, "/server/port").(*Number).Raw(); got != "9090" {
		t.Errorf("/server/port = %s, want 9090", got)
	}

	// The ancestors of the edited node are rebuilt, and nothing else is.
	if nodeAt(t, res.Root, "/server") == Node(d.server) {
		t.Error("/server was shared, want it rebuilt: it holds the replaced value")
	}

	if nodeAt(t, res.Root, "/server/host") != Node(d.host) {
		t.Error("/server/host was rebuilt, want it shared")
	}

	if nodeAt(t, res.Root, "/features") != Node(d.features) {
		t.Error("/features was rebuilt, want it shared")
	}

	// The tree that went in is a tree the history still holds, so it has to
	// come out of the edit as it was.
	if nodeAt(t, d.root, "/server/port") != Node(d.port) {
		t.Error("the original tree changed")
	}
}

func TestSetValueAtTheRootReplacesTheWholeDocument(t *testing.T) {
	t.Parallel()

	d := newTree(t)
	replacement := str(t, "nothing left")

	res, err := SetValue(d.root, Path{}, replacement)
	if err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	if res.Root != Node(replacement) {
		t.Errorf("Root = %v, want the replacement itself", res.Root.Kind())
	}

	if !res.Cursor.IsRoot() {
		t.Errorf("Cursor = %q, want the root", res.Cursor)
	}
}

func TestEditingAMemberKeepsTheCommentsAroundIt(t *testing.T) {
	t.Parallel()

	// Nothing can attach a comment to a node yet, but path copying is where
	// they would be dropped once something can. The member is rebuilt whole so
	// that its trivia travels with it.
	trivia := NewTrivia([]Comment{{Text: " the listening port"}}, nil)
	root := obj(t,
		Member{Key: "port", Value: NewNumber("8080"), Trivia: trivia},
		Member{Key: "host", Value: str(t, "localhost")},
	)

	tests := []struct {
		name string
		edit func() (EditResult, error)
		key  string
	}{
		{
			name: "replacing its value",
			edit: func() (EditResult, error) {
				return SetValue(root, at(t, "/port"), NewNumber("9090"))
			},
			key: "port",
		},
		{
			name: "renaming it",
			edit: func() (EditResult, error) {
				return Rename(root, at(t, "/port"), "listen")
			},
			key: "listen",
		},
		{
			name: "adding a sibling",
			edit: func() (EditResult, error) {
				return Insert(root, Path{}, 2, Member{
					Key:   "debug",
					Value: NewBool(false),
				})
			},
			key: "port",
		},
		{
			name: "deleting a sibling",
			edit: func() (EditResult, error) {
				return Delete(root, at(t, "/host"))
			},
			key: "port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res, err := tt.edit()
			if err != nil {
				t.Fatalf("edit: %v", err)
			}

			o := res.Root.(*Object)

			i, ok := o.IndexOf(tt.key)
			if !ok {
				t.Fatalf("the member %q is gone", tt.key)
			}

			if o.At(i).Trivia.IsEmpty() {
				t.Errorf("the comments on %q were dropped", tt.key)
			}
		})
	}
}

func TestEditsRefuseWhatTheDocumentCannotTake(t *testing.T) {
	t.Parallel()

	d := newTree(t)

	tests := []struct {
		name   string
		edit   func() (EditResult, error)
		reason string
	}{
		{
			name: "setting a value at a key that is not there",
			edit: func() (EditResult, error) {
				return SetValue(d.root, at(t, "/nope"), NewNull())
			},
			reason: "no such node",
		},
		{
			name: "setting a value beneath a scalar",
			edit: func() (EditResult, error) {
				return SetValue(d.root, at(t, "/debug/deeper"), NewNull())
			},
			reason: "no such node",
		},
		{
			name: "setting a value past the end of an array",
			edit: func() (EditResult, error) {
				return SetValue(d.root, at(t, "/features/3"), NewNull())
			},
			reason: "no such node",
		},
		{
			name: "renaming the root",
			edit: func() (EditResult, error) {
				return Rename(d.root, Path{}, "anything")
			},
			reason: "not an object member",
		},
		{
			name: "renaming an array element",
			edit: func() (EditResult, error) {
				return Rename(d.root, at(t, "/features/0"), "anything")
			},
			reason: "not an object member",
		},
		{
			name: "renaming a member that is not there",
			edit: func() (EditResult, error) {
				return Rename(d.root, at(t, "/server/nope"), "anything")
			},
			reason: "no such node",
		},
		{
			name: "inserting into a scalar",
			edit: func() (EditResult, error) {
				return Insert(d.root, at(t, "/debug"), 0, Member{
					Key:   "k",
					Value: NewNull(),
				})
			},
			reason: "not a container",
		},
		{
			name: "inserting into a container that is not there",
			edit: func() (EditResult, error) {
				return Insert(d.root, at(t, "/nope"), 0, Member{
					Key:   "k",
					Value: NewNull(),
				})
			},
			reason: "no such node",
		},
		{
			name: "inserting past the end of an object",
			edit: func() (EditResult, error) {
				return Insert(d.root, at(t, "/server"), 3, Member{
					Key:   "k",
					Value: NewNull(),
				})
			},
			reason: "index out of range",
		},
		{
			name: "inserting past the end of an array",
			edit: func() (EditResult, error) {
				return Insert(d.root, at(t, "/features"), 4, Member{
					Value: NewNull(),
				})
			},
			reason: "index out of range",
		},
		{
			name: "giving an array element a key",
			edit: func() (EditResult, error) {
				return Insert(d.root, at(t, "/features"), 0, Member{
					Key:   "named",
					Value: NewNull(),
				})
			},
			reason: "an array element cannot have a key",
		},
		{
			name: "deleting the root",
			edit: func() (EditResult, error) {
				return Delete(d.root, Path{})
			},
			reason: "the root cannot be deleted",
		},
		{
			name: "deleting a node that is not there",
			edit: func() (EditResult, error) {
				return Delete(d.root, at(t, "/nope"))
			},
			reason: "no such node",
		},
		{
			name: "changing the type of a node that is not there",
			edit: func() (EditResult, error) {
				return ChangeType(d.root, at(t, "/nope"), KindString)
			},
			reason: "no such node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res, err := tt.edit()

			var invalid *EditError
			if !errors.As(err, &invalid) {
				t.Fatalf("error = %v, want *EditError", err)
			}

			if invalid.Reason != tt.reason {
				t.Errorf("Reason = %q, want %q", invalid.Reason, tt.reason)
			}

			if res.Root != nil {
				t.Error("a refused edit came back with a tree")
			}
		})
	}
}

func TestEditsRefuseARepeatedKey(t *testing.T) {
	t.Parallel()

	d := newTree(t)

	tests := []struct {
		name string
		edit func() (EditResult, error)
	}{
		{
			name: "renaming a member onto one of its siblings",
			edit: func() (EditResult, error) {
				return Rename(d.root, at(t, "/server/host"), "port")
			},
		},
		{
			name: "adding a member with a key already there",
			edit: func() (EditResult, error) {
				return Insert(d.root, at(t, "/server"), 2, Member{
					Key:   "host",
					Value: NewNull(),
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The check comes from the constructor, so it holds for every edit
			// that rebuilds an object rather than being written per operation.
			res, err := tt.edit()

			var dup *DuplicateKeyError
			if !errors.As(err, &dup) {
				t.Fatalf("error = %v, want *DuplicateKeyError", err)
			}

			if dup.Key != "host" && dup.Key != "port" {
				t.Errorf("Key = %q, want the repeated one", dup.Key)
			}

			if res.Root != nil {
				t.Error("a refused edit came back with a tree")
			}
		})
	}
}

func TestRenameMovesTheMemberAndWhatIsUnderIt(t *testing.T) {
	t.Parallel()

	d := newTree(t)

	res, err := Rename(d.root, at(t, "/server"), "listener")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if !res.Cursor.Equal(at(t, "/listener")) {
		t.Errorf("Cursor = %q, want /listener", res.Cursor)
	}

	// The member keeps its place: renaming is not a reordering.
	if got := res.Root.(*Object).At(0).Key; got != "listener" {
		t.Errorf("the first member is %q, want listener", got)
	}

	// The value is shared, so the subtree moved without being copied.
	if nodeAt(t, res.Root, "/listener") != Node(d.server) {
		t.Error("/listener was rebuilt, want the subtree shared")
	}

	if nodeAt(t, res.Root, "/listener/port") != Node(d.port) {
		t.Error("/listener/port is not the node that moved")
	}

	want := [][2]string{{"/server", "/listener"}}
	if got := pairs(res.Renames); !slices.Equal(got, want) {
		t.Errorf("Renames = %v, want %v", got, want)
	}
}

func TestRenameMovesOnlyWhatItsKeyBegins(t *testing.T) {
	t.Parallel()

	// "/a" is a prefix of "/ab" as text and of nothing as a path. A rename that
	// went by text alone would carry the wrong member.
	root := obj(t,
		Member{Key: "a", Value: obj(t, Member{
			Key:   "x",
			Value: NewNull(),
		})},
		Member{Key: "ab", Value: NewNull()},
	)

	res, err := Rename(root, at(t, "/a"), "z")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	pointers := []string{"/a", "/a/x", "/ab"}
	want := []string{"/z", "/z/x", "/ab"}

	if got := rewriteTogether(res.Renames, pointers); !slices.Equal(got, want) {
		t.Errorf("moved %v, want %v", got, want)
	}
}

func TestDeleteSaysWhereToStandAfterwards(t *testing.T) {
	t.Parallel()

	d := newTree(t)

	tests := []struct {
		name    string
		target  string
		cursor  string
		renames [][2]string
	}{
		{
			// The next sibling, so that d can be held down to clear a run of
			// members from the same row.
			name:   "a member with one after it",
			target: "/server/host",
			cursor: "/server/port",
		},
		{
			name:   "the last member",
			target: "/server/port",
			cursor: "/server/host",
		},
		{
			// The element that followed has moved down into this position.
			name:    "an element with one after it",
			target:  "/features/0",
			cursor:  "/features/0",
			renames: [][2]string{{"/features/1", "/features/0"}, {"/features/2", "/features/1"}},
		},
		{
			name:    "an element in the middle",
			target:  "/features/1",
			cursor:  "/features/1",
			renames: [][2]string{{"/features/2", "/features/1"}},
		},
		{
			name:   "the last element",
			target: "/features/2",
			cursor: "/features/1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := at(t, tt.target)

			res, err := Delete(d.root, p)
			if err != nil {
				t.Fatalf("Delete: %v", err)
			}

			if !res.Cursor.Equal(at(t, tt.cursor)) {
				t.Errorf("Cursor = %q, want %q", res.Cursor, tt.cursor)
			}

			if got := pairs(res.Renames); !slices.Equal(got, tt.renames) {
				t.Errorf("Renames = %v, want %v", got, tt.renames)
			}

			// A deleted array element leaves its position occupied by the one
			// that followed, so the count is what says it is gone.
			container := p.Parent().String()

			before := childCount(t, nodeAt(t, d.root, container))
			if got := childCount(t, nodeAt(t, res.Root, container)); got != before-1 {
				t.Errorf("%s holds %d children, want %d", container, got, before-1)
			}
		})
	}
}

func TestDeletingTheOnlyChildLeavesTheCursorOnTheContainer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		root   Node
		target string
	}{
		{
			name: "the only member",
			root: obj(t, Member{
				Key:   "server",
				Value: obj(t, Member{Key: "host", Value: str(t, "localhost")}),
			}),
			target: "/server/host",
		},
		{
			name: "the only element",
			root: obj(t, Member{
				Key:   "features",
				Value: NewArray([]Node{str(t, "search")}),
			}),
			target: "/features/0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := at(t, tt.target)

			res, err := Delete(tt.root, p)
			if err != nil {
				t.Fatalf("Delete: %v", err)
			}

			if !res.Cursor.Equal(p.Parent()) {
				t.Errorf("Cursor = %q, want %q", res.Cursor, p.Parent())
			}
		})
	}
}

func TestInsertPutsTheChildWhereItWasAsked(t *testing.T) {
	t.Parallel()

	d := newTree(t)

	t.Run("at the end of an object", func(t *testing.T) {
		t.Parallel()

		res, err := Insert(d.root, at(t, "/server"), 2, Member{
			Key:   "timeout",
			Value: NewNumber("30"),
		})
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}

		if !res.Cursor.Equal(at(t, "/server/timeout")) {
			t.Errorf("Cursor = %q, want /server/timeout", res.Cursor)
		}

		if got := nodeAt(t, res.Root, "/server").(*Object).At(2).Key; got != "timeout" {
			t.Errorf("the third member is %q, want timeout", got)
		}

		// The keys of an object do not depend on their order, so nothing moved.
		if len(res.Renames) != 0 {
			t.Errorf("Renames = %v, want none", pairs(res.Renames))
		}
	})

	t.Run("before the members already there", func(t *testing.T) {
		t.Parallel()

		res, err := Insert(d.root, at(t, "/server"), 0, Member{
			Key:   "timeout",
			Value: NewNumber("30"),
		})
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}

		o := nodeAt(t, res.Root, "/server").(*Object)

		var keys []string
		for _, m := range o.All() {
			keys = append(keys, m.Key)
		}

		want := []string{"timeout", "host", "port"}
		if !slices.Equal(keys, want) {
			t.Errorf("keys = %v, want %v", keys, want)
		}
	})

	t.Run("in the middle of an array", func(t *testing.T) {
		t.Parallel()

		res, err := Insert(d.root, at(t, "/features"), 1, Member{
			Value: str(t, "new"),
		})
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}

		if !res.Cursor.Equal(at(t, "/features/1")) {
			t.Errorf("Cursor = %q, want /features/1", res.Cursor)
		}

		if nodeAt(t, res.Root, "/features/2") != Node(d.second) {
			t.Error("the element that was second did not move to third")
		}

		want := [][2]string{{"/features/1", "/features/2"}, {"/features/2", "/features/3"}}
		if got := pairs(res.Renames); !slices.Equal(got, want) {
			t.Errorf("Renames = %v, want %v", got, want)
		}

		// The tree that went in still has three.
		if got := d.features.Len(); got != 3 {
			t.Errorf("the original array holds %d elements, want 3", got)
		}
	})

	t.Run("at the end of an array", func(t *testing.T) {
		t.Parallel()

		res, err := Insert(d.root, at(t, "/features"), 3, Member{
			Value: str(t, "new"),
		})
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}

		if !res.Cursor.Equal(at(t, "/features/3")) {
			t.Errorf("Cursor = %q, want /features/3", res.Cursor)
		}

		if len(res.Renames) != 0 {
			t.Errorf("Renames = %v, want none: nothing follows the new element", pairs(res.Renames))
		}
	})

	t.Run("into an empty container", func(t *testing.T) {
		t.Parallel()

		res, err := Insert(d.root, at(t, "/empty"), 0, Member{
			Key:   "first",
			Value: NewNull(),
		})
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}

		if !res.Cursor.Equal(at(t, "/empty/first")) {
			t.Errorf("Cursor = %q, want /empty/first", res.Cursor)
		}
	})
}

func TestChangeTypeCarriesTheValueOverAndDropsTheChildren(t *testing.T) {
	t.Parallel()

	d := newTree(t)

	tests := []struct {
		name   string
		target string
		kind   Kind
		want   string
	}{
		{
			name:   "a number seen as text",
			target: "/server/port",
			kind:   KindString,
			want:   `"8080"`,
		},
		{
			name:   "text that spells no number",
			target: "/server/host",
			kind:   KindNumber,
			want:   "0",
		},
		{
			name:   "a container made a scalar",
			target: "/server",
			kind:   KindString,
			want:   `""`,
		},
		{
			name:   "a container made another container",
			target: "/server",
			kind:   KindArray,
			want:   "[]",
		},
		{
			name:   "the root",
			target: "",
			kind:   KindArray,
			want:   "[]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := at(t, tt.target)

			res, err := ChangeType(d.root, p, tt.kind)
			if err != nil {
				t.Fatalf("ChangeType: %v", err)
			}

			if !res.Cursor.Equal(p) {
				t.Errorf("Cursor = %q, want %q", res.Cursor, p)
			}

			if got := describe(t, nodeAt(t, res.Root, tt.target)); got != tt.want {
				t.Errorf("%s = %s, want %s", tt.target, got, tt.want)
			}

			// The children go with the type, and go nowhere else: the tree
			// that went in still holds them for undo to come back to.
			if got := CountDescendants(d.server); got != 2 {
				t.Errorf("the original /server holds %d nodes, want 2", got)
			}
		})
	}
}

func TestAnEditThatChangesNothingReturnsTheSameTree(t *testing.T) {
	t.Parallel()

	d := newTree(t)

	// Insert and Delete are missing because neither has a form that changes
	// nothing: a document with a node added or taken away is a different one.
	tests := []struct {
		name string
		edit func() (EditResult, error)
	}{
		{
			name: "a number typed back as it was",
			edit: func() (EditResult, error) {
				return SetValue(d.root, at(t, "/server/port"), NewNumber("8080"))
			},
		},
		{
			name: "a string typed back as it was",
			edit: func() (EditResult, error) {
				return SetValue(d.root, at(t, "/server/host"), str(t, "localhost"))
			},
		},
		{
			name: "a boolean set to what it already is",
			edit: func() (EditResult, error) {
				return SetValue(d.root, at(t, "/debug"), NewBool(false))
			},
		},
		{
			name: "a member renamed to its own key",
			edit: func() (EditResult, error) {
				return Rename(d.root, at(t, "/server/host"), "host")
			},
		},
		{
			name: "the type it already has",
			edit: func() (EditResult, error) {
				return ChangeType(d.root, at(t, "/server/port"), KindNumber)
			},
		},
		{
			name: "the type the root already has",
			edit: func() (EditResult, error) {
				return ChangeType(d.root, Path{}, KindObject)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res, err := tt.edit()
			if err != nil {
				t.Fatalf("edit: %v", err)
			}

			// One pointer comparison stands for three things above: no version
			// is pushed, the unsaved mark stays off, and nothing follows paths
			// that did not move.
			if res.Root != Node(d.root) {
				t.Error("a new tree came back from an edit that changed nothing")
			}

			if len(res.Renames) != 0 {
				t.Errorf("Renames = %v, want none", pairs(res.Renames))
			}
		})
	}
}

func TestEveryEditLeavesTheCursorOnANodeThatIsThere(t *testing.T) {
	t.Parallel()

	d := newTree(t)

	tests := []struct {
		name string
		edit func() (EditResult, error)
	}{
		{"setting a value", func() (EditResult, error) {
			return SetValue(d.root, at(t, "/server/port"), NewNumber("1"))
		}},
		{"renaming a member", func() (EditResult, error) {
			return Rename(d.root, at(t, "/server/port"), "listen")
		}},
		{"adding a member", func() (EditResult, error) {
			return Insert(d.root, at(t, "/server"), 0, Member{
				Key:   "timeout",
				Value: NewNumber("30"),
			})
		}},
		{"adding an element", func() (EditResult, error) {
			return Insert(d.root, at(t, "/features"), 1, Member{
				Value: NewNull(),
			})
		}},
		{"deleting a member", func() (EditResult, error) {
			return Delete(d.root, at(t, "/server/port"))
		}},
		{"deleting an element", func() (EditResult, error) {
			return Delete(d.root, at(t, "/features/2"))
		}},
		{"deleting the only child", func() (EditResult, error) {
			return Delete(d.root, at(t, "/features/0"))
		}},
		{"changing a type", func() (EditResult, error) {
			return ChangeType(d.root, at(t, "/server"), KindNull)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res, err := tt.edit()
			if err != nil {
				t.Fatalf("edit: %v", err)
			}

			// The version an edit pushes is read back with this cursor, so a
			// path that does not resolve would be a position undo cannot
			// restore.
			if _, ok := Resolve(res.Root, res.Cursor); !ok {
				t.Errorf("Cursor %q does not resolve in the tree the edit produced", res.Cursor)
			}
		})
	}
}

func TestRewritePointerMovesAPointerWithThePathAboveIt(t *testing.T) {
	t.Parallel()

	maps := []PathMap{
		{From: at(t, "/a"), To: at(t, "/z")},
		{From: at(t, "/features/1"), To: at(t, "/features/2")},
	}

	tests := []struct {
		name    string
		pointer string
		want    string
	}{
		{name: "the path that moved", pointer: "/a", want: "/z"},
		{name: "a child of it", pointer: "/a/x", want: "/z/x"},
		{name: "a grandchild of it", pointer: "/a/x/y", want: "/z/x/y"},
		{
			// "/a" is a prefix of "/ab" as text and of nothing as a path.
			name:    "a sibling whose key begins the same way",
			pointer: "/ab",
			want:    "/ab",
		},
		{name: "a path no map covers", pointer: "/b", want: "/b"},
		{name: "the root", pointer: "", want: ""},
		{name: "an element that moved", pointer: "/features/1", want: "/features/2"},
		{name: "beneath an element that moved", pointer: "/features/1/name", want: "/features/2/name"},
		{
			// The token is "10", not "1" followed by "0".
			name:    "an element whose index begins the same way",
			pointer: "/features/10",
			want:    "/features/10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := RewritePointer(maps, tt.pointer); got != tt.want {
				t.Errorf("RewritePointer(%q) = %q, want %q", tt.pointer, got, tt.want)
			}
		})
	}
}

func TestPointerRemovedCoversTheWholeSubtree(t *testing.T) {
	t.Parallel()

	removed := []Path{at(t, "/server")}

	tests := []struct {
		name    string
		pointer string
		want    bool
	}{
		{name: "the subtree that went", pointer: "/server", want: true},
		{name: "something inside it", pointer: "/server/host", want: true},
		{name: "deep inside it", pointer: "/server/tls/cert", want: true},
		{name: "a sibling", pointer: "/servers", want: false},
		{name: "somewhere else", pointer: "/features/0", want: false},
		{name: "the root", pointer: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := PointerRemoved(removed, tt.pointer); got != tt.want {
				t.Errorf("PointerRemoved(%q) = %v, want %v", tt.pointer, got, tt.want)
			}
		})
	}
}

func TestRemovingTheRootCoversEveryPointer(t *testing.T) {
	t.Parallel()

	// Replacing the root discards the whole document, and the root is above
	// everything: nothing a set holds can survive it.
	removed := []Path{{}}

	for _, pointer := range []string{"", "/a", "/a/b", "/features/0"} {
		if !PointerRemoved(removed, pointer) {
			t.Errorf("PointerRemoved(%q) = false, want true", pointer)
		}
	}
}

func TestAnEditSaysWhichSubtreesItTookOut(t *testing.T) {
	t.Parallel()

	d := newTree(t)

	tests := []struct {
		name string
		edit func() (EditResult, error)
		want []string
	}{
		{
			name: "deleting a scalar",
			edit: func() (EditResult, error) {
				return Delete(d.root, at(t, "/server/port"))
			},
			want: []string{"/server/port"},
		},
		{
			name: "deleting a container",
			edit: func() (EditResult, error) {
				return Delete(d.root, at(t, "/server"))
			},
			want: []string{"/server"},
		},
		{
			name: "changing a container to a scalar",
			edit: func() (EditResult, error) {
				return ChangeType(d.root, at(t, "/server"), KindString)
			},
			want: []string{"/server"},
		},
		{
			name: "changing an empty container",
			edit: func() (EditResult, error) {
				return ChangeType(d.root, at(t, "/empty"), KindString)
			},
			want: nil,
		},
		{
			name: "changing a scalar",
			edit: func() (EditResult, error) {
				return ChangeType(d.root, at(t, "/server/port"), KindString)
			},
			want: nil,
		},
		{
			name: "replacing a container with a scalar",
			edit: func() (EditResult, error) {
				return SetValue(d.root, at(t, "/server"), NewNull())
			},
			want: []string{"/server"},
		},
		{
			name: "replacing a scalar",
			edit: func() (EditResult, error) {
				return SetValue(d.root, at(t, "/server/port"), NewNumber("1"))
			},
			want: nil,
		},
		{
			// The member and everything under it moved; nothing went away.
			name: "renaming a member",
			edit: func() (EditResult, error) {
				return Rename(d.root, at(t, "/server"), "listener")
			},
			want: nil,
		},
		{
			name: "adding an element",
			edit: func() (EditResult, error) {
				return Insert(d.root, at(t, "/features"), 0, Member{
					Value: NewNull(),
				})
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res, err := tt.edit()
			if err != nil {
				t.Fatalf("edit: %v", err)
			}

			if got := paths(res.Removed); !slices.Equal(got, tt.want) {
				t.Errorf("Removed = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenamesMoveEverythingBeneathThePathThatMoved(t *testing.T) {
	t.Parallel()

	d := newTree(t)

	// The pointers a collapsed set might hold over this array, including one
	// that is only a prefix as text.
	pointers := []string{
		"/features",
		"/features/0",
		"/features/1",
		"/features/1/name",
		"/features/2",
		"/features/2/name",
	}

	t.Run("an element removed pulls the rest down", func(t *testing.T) {
		t.Parallel()

		res, err := Delete(d.root, at(t, "/features/1"))
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}

		// The deleted element has no rename, because it went nowhere. Its path
		// still resolves afterwards — to the element that moved up into its
		// place — so a holder of paths has to be told, and Removed is how.
		if got := paths(res.Removed); !slices.Equal(got, []string{"/features/1"}) {
			t.Errorf("Removed = %v, want [/features/1]", got)
		}

		want := []string{
			"/features",
			"/features/0",
			"/features/1",
			"/features/1/name",
		}

		got := rewriteTogether(res.Renames, dropRemoved(res.Removed, pointers))
		if !slices.Equal(got, want) {
			t.Errorf("moved %v, want %v", got, want)
		}

		// The other order drops the element that has just arrived at
		// "/features/1" along with the one that left it, which is why Removed
		// is defined to apply first.
		reversed := dropRemoved(res.Removed, rewriteTogether(res.Renames, pointers))
		if slices.Equal(reversed, want) {
			t.Error("dropping the removed paths last gave the right answer, " +
				"so this input no longer shows that it has to come first")
		}
	})

	t.Run("an element added pushes the rest up", func(t *testing.T) {
		t.Parallel()

		res, err := Insert(d.root, at(t, "/features"), 1, Member{
			Value: NewNull(),
		})
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}

		want := []string{
			"/features",
			"/features/0",
			"/features/2",
			"/features/2/name",
			"/features/3",
			"/features/3/name",
		}

		got := rewriteTogether(res.Renames, pointers)
		if !slices.Equal(got, want) {
			t.Errorf("moved %v, want %v", got, want)
		}

		// The same maps applied one after another carry "/features/1" through
		// "/features/2" and on to "/features/3". The maps of one edit describe
		// a single move of the whole set, which is why they are defined to
		// apply at once.
		if inTurn := rewriteInTurn(res.Renames, pointers); slices.Equal(inTurn, want) {
			t.Error("applying the renames in turn gave the right answer, " +
				"so this input no longer shows that they have to apply at once")
		}
	})
}

// Nodes can carry comments in places no public constructor accepts yet. The
// fixtures set that state directly so edits cannot silently drop it before a
// parser can produce the same trees.
func TestRebuildingAContainerKeepsTheCommentsAroundIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		edit func(root *Object) (EditResult, error)
	}{
		{
			name: "replacing an element",
			edit: func(root *Object) (EditResult, error) {
				return SetValue(root, path(KeySegment("features"), IndexSegment(1)), NewNumber("1"))
			},
		},
		{
			name: "adding an element",
			edit: func(root *Object) (EditResult, error) {
				return Insert(root, path(KeySegment("features")), 0, Member{Value: NewNull()})
			},
		},
		{
			name: "removing an element",
			edit: func(root *Object) (EditResult, error) {
				return Delete(root, path(KeySegment("features"), IndexSegment(0)))
			},
		},
		{
			name: "renaming the member holding it",
			edit: func(root *Object) (EditResult, error) {
				return Rename(root, path(KeySegment("features")), "abilities")
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
	element := path(KeySegment("features"), IndexSegment(0))

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
	element := path(KeySegment("features"), IndexSegment(0))

	res, err := ChangeType(root, element, KindString)
	if err != nil {
		t.Fatalf("ChangeType: %v", err)
	}

	if res.Root != Node(root) {
		t.Error("a new tree came back from a change to the type already there")
	}
}
