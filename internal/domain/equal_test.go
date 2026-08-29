package domain

import "testing"

// A tree built twice is two sets of pointers holding one document, which is
// what a document read back from disk is. Nothing but a deep comparison can
// say that they are the same.
func TestEqualHoldsForADocumentBuiltTwice(t *testing.T) {
	t.Parallel()

	a, b := newTree(t).root, newTree(t).root

	if a == Node(b) {
		t.Fatal("the two trees share a root, so nothing deep is being compared")
	}

	if !Equal(a, b) {
		t.Error("Equal = false for two builds of the same document, want true")
	}
}

// The comparison is what pino trusts before overwriting a file, so what it
// has to notice is every way two documents can differ.
func TestEqualDistinguishesDocumentsThatDiffer(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		a, b func(t *testing.T) Node
	}{
		"a kind": {
			a: func(t *testing.T) Node { return str(t, "1") },
			b: func(*testing.T) Node { return NewNumber("1") },
		},
		"a string": {
			a: func(t *testing.T) Node { return str(t, "localhost") },
			b: func(t *testing.T) Node { return str(t, "127.0.0.1") },
		},
		"a boolean": {
			a: func(*testing.T) Node { return NewBool(true) },
			b: func(*testing.T) Node { return NewBool(false) },
		},

		// The same quantity, spelled two ways. pino writes back the literal it
		// read, so these are two documents.
		"the spelling of a number": {
			a: func(*testing.T) Node { return NewNumber("1.50") },
			b: func(*testing.T) Node { return NewNumber("1.5") },
		},
		"the spelling of an exponent": {
			a: func(*testing.T) Node { return NewNumber("1e3") },
			b: func(*testing.T) Node { return NewNumber("1000") },
		},
		"how many members an object has": {
			a: func(t *testing.T) Node { return obj(t, Member{Key: "a", Value: NewNumber("1")}) },
			b: func(t *testing.T) Node {
				return obj(t,
					Member{Key: "a", Value: NewNumber("1")},
					Member{Key: "b", Value: NewNumber("2")},
				)
			},
		},
		"a key": {
			a: func(t *testing.T) Node { return obj(t, Member{Key: "host", Value: NewNumber("1")}) },
			b: func(t *testing.T) Node { return obj(t, Member{Key: "port", Value: NewNumber("1")}) },
		},

		// Members are written back in the order they were read, so an object
		// holding the same pairs in another order is another file.
		"the order of members": {
			a: func(t *testing.T) Node {
				return obj(t,
					Member{Key: "a", Value: NewNumber("1")},
					Member{Key: "b", Value: NewNumber("2")},
				)
			},
			b: func(t *testing.T) Node {
				return obj(t,
					Member{Key: "b", Value: NewNumber("2")},
					Member{Key: "a", Value: NewNumber("1")},
				)
			},
		},
		"how many elements an array has": {
			a: func(*testing.T) Node { return NewArray([]Node{NewNumber("1")}) },
			b: func(*testing.T) Node { return NewArray([]Node{NewNumber("1"), NewNumber("2")}) },
		},
		"the order of elements": {
			a: func(*testing.T) Node { return NewArray([]Node{NewNumber("1"), NewNumber("2")}) },
			b: func(*testing.T) Node { return NewArray([]Node{NewNumber("2"), NewNumber("1")}) },
		},
		"a value deep in the document": {
			a: func(t *testing.T) Node { return newTree(t).root },
			b: func(t *testing.T) Node {
				d := newTree(t)

				res, err := SetValue(d.root, at(t, "/server/port"), NewNumber("8081"))
				if err != nil {
					t.Fatalf("SetValue: %v", err)
				}

				return res.Root
			},
		},
		"an empty container against a missing one": {
			a: func(t *testing.T) Node { return obj(t, Member{Key: "a", Value: obj(t)}) },
			b: func(t *testing.T) Node { return obj(t, Member{Key: "a", Value: NewArray(nil)}) },
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			a, b := tc.a(t), tc.b(t)

			if Equal(a, b) {
				t.Errorf("Equal = true for documents differing in %s, want false", name)
			}

			if !Equal(a, a) || !Equal(b, b) {
				t.Error("a document is not equal to itself")
			}
		})
	}
}

// A missing value is a mistake in the caller, and two of them are equal by
// pointer. Answering "the same document" for one would let a save go ahead on
// the strength of a comparison that read nothing at all.
func TestEqualPanicsOnANodeThatIsNotThere(t *testing.T) {
	t.Parallel()

	nodes := typedNils()
	nodes["nothing at all"] = nil

	real := func(t *testing.T) Node {
		t.Helper()

		return obj(t, Member{Key: "a", Value: NewNumber("1")})
	}

	for name, missing := range nodes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cases := map[string][2]Node{
				"on the left":  {missing, real(t)},
				"on the right": {real(t), missing},

				// The pair that used to be answered by pointer identity
				// before either side was read.
				"on both sides": {missing, missing},
			}

			for where, pair := range cases {
				t.Run(where, func(t *testing.T) {
					t.Parallel()

					defer func() {
						if recover() == nil {
							t.Errorf("Equal with a missing %s returned normally, want a panic", name)
						}
					}()

					Equal(pair[0], pair[1])
				})
			}
		})
	}
}

// Comments are empty in every document pino accepts today. They are compared
// anyway, so that an encoder which drops one when JSONC arrives is caught by
// the check already standing in front of every save.
func TestEqualComparesTheCommentsAroundValues(t *testing.T) {
	t.Parallel()

	commentedRoot, commentedArray := commented(t)

	plain, err := NewObject([]Member{{Key: "features", Value: NewArray([]Node{
		str(t, "search"),
		str(t, "history"),
	})}})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	if Equal(commentedRoot, plain) {
		t.Error("Equal = true for a document whose comments were dropped, want false")
	}

	elsewhere, _ := commented(t)
	if !Equal(commentedRoot, elsewhere) {
		t.Error("Equal = false for two builds of the same commented document, want true")
	}

	// A comment on a member sits on the pair rather than on the value, so it
	// is the one the recursion does not reach on its own.
	tagged, err := NewObject([]Member{{
		Key:    "features",
		Value:  commentedArray,
		Trivia: note(" what the tool can do"),
	}})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	untagged, err := NewObject([]Member{{Key: "features", Value: commentedArray}})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	if Equal(tagged, untagged) {
		t.Error("Equal = true for a member whose comment was dropped, want false")
	}
}

func TestEqualComparesCommentsInsideAContainer(t *testing.T) {
	t.Parallel()

	inside := NewTrivia(nil, nil, []Comment{comment(t, " pending", false, true)})
	commented := WithTrivia(obj(t), inside)
	plain := obj(t)

	if Equal(commented, plain) {
		t.Error("Equal = true for containers differing only in inside comments, want false")
	}

	elsewhere := WithTrivia(obj(t), inside)
	if !Equal(commented, elsewhere) {
		t.Error("Equal = false for containers with the same inside comments, want true")
	}
}

// An edit rebuilds the nodes along one path and shares everything else, which
// is the shape of every tree pino compares: part of it is the very tree it is
// being compared with.
func TestEqualLooksThroughSharedSubtrees(t *testing.T) {
	t.Parallel()

	d := newTree(t)

	res, err := SetValue(d.root, at(t, "/server/port"), NewNumber("8081"))
	if err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	if nodeAt(t, res.Root, "/features") != Node(d.features) {
		t.Fatal("the edit rebuilt the untouched subtree, so nothing shared is being compared")
	}

	want := newTree(t)

	changed, err := SetValue(want.root, at(t, "/server/port"), NewNumber("8081"))
	if err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	if !Equal(res.Root, changed.Root) {
		t.Error("Equal = false for two documents edited the same way, want true")
	}

	if Equal(res.Root, d.root) {
		t.Error("Equal = true for a document and its edited version, want false")
	}
}
