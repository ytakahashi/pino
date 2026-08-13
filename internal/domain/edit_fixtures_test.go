package domain

import (
	"testing"
)

// tree is a document with each of its nodes named, so that a test can assert
// which subtrees an edit rebuilt and which ones it shared.
//
//	{
//	  "server":   { "host": "localhost", "port": 8080 },
//	  "features": [ {"name": "first"}, {"name": "second"}, {"name": "third"} ],
//	  "debug":    false
//	}
type tree struct {
	root     *Object
	server   *Object
	host     *String
	port     *Number
	features *Array
	first    *Object
	second   *Object
	third    *Object
	debug    *Bool
	empty    *Object
}

func newTree(t *testing.T) tree {
	t.Helper()

	var d tree

	d.host = str(t, "localhost")
	d.port = NewNumber("8080")
	d.server = obj(t,
		Member{Key: "host", Value: d.host},
		Member{Key: "port", Value: d.port},
	)

	// The elements hold a child each, so that a rename of an element can be
	// checked to carry what is underneath it.
	d.first = obj(t, Member{Key: "name", Value: str(t, "first")})
	d.second = obj(t, Member{Key: "name", Value: str(t, "second")})
	d.third = obj(t, Member{Key: "name", Value: str(t, "third")})
	d.features = NewArray([]Node{d.first, d.second, d.third})

	d.debug = NewBool(false)
	d.empty = obj(t)

	d.root = obj(t,
		Member{Key: "server", Value: d.server},
		Member{Key: "features", Value: d.features},
		Member{Key: "debug", Value: d.debug},
		Member{Key: "empty", Value: d.empty},
	)

	return d
}

// at is the path a JSON Pointer names, for a test that wants to read as the
// document does.
func at(t *testing.T, pointer string) Path {
	t.Helper()

	p, err := ParsePointer(pointer)
	if err != nil {
		t.Fatalf("ParsePointer(%q): %v", pointer, err)
	}

	return p
}

// nodeAt is the node a pointer names, failing the test when the path leads
// nowhere.
func nodeAt(t *testing.T, root Node, pointer string) Node {
	t.Helper()

	n, ok := Resolve(root, at(t, pointer))
	if !ok {
		t.Fatalf("%q does not resolve", pointer)
	}

	return n
}

// childCount is how many children a container holds directly.
func childCount(t *testing.T, n Node) int {
	t.Helper()

	switch n.Kind() {
	case KindObject:
		return n.(*Object).Len()
	case KindArray:
		return n.(*Array).Len()
	case KindString, KindNumber, KindBool, KindNull:
		t.Fatalf("childCount: %v holds no children", n.Kind())

		return 0
	default:
		t.Fatalf("childCount: %v holds no children", n.Kind())

		return 0
	}
}

// pairs is the renames written as pointers, which is how a test spells what it
// expects without building paths by hand.
func pairs(maps []PathMap) [][2]string {
	out := make([][2]string, 0, len(maps))
	for _, m := range maps {
		out = append(out, [2]string{m.From.String(), m.To.String()})
	}

	return out
}

// dropRemoved drops the pointers that named something the edit took out,
// which is the step that has to come before the renames are applied.
func dropRemoved(removed []Path, pointers []string) []string {
	out := make([]string, 0, len(pointers))

	for _, ptr := range pointers {
		if !PointerRemoved(removed, ptr) {
			out = append(out, ptr)
		}
	}

	return out
}

// paths is the removed paths written as pointers.
func paths(ps []Path) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.String())
	}

	return out
}

// rewriteTogether moves pointers the way the collapsed set will: every pointer
// is looked at once, against the whole set of maps.
func rewriteTogether(maps []PathMap, pointers []string) []string {
	out := make([]string, len(pointers))
	for i, ptr := range pointers {
		out[i] = RewritePointer(maps, ptr)
	}

	return out
}

// rewriteInTurn moves pointers by applying each map to the whole set before
// looking at the next one. It exists so that a test can show this is not the
// same thing as rewriteTogether.
func rewriteInTurn(maps []PathMap, pointers []string) []string {
	out := append([]string(nil), pointers...)

	for _, m := range maps {
		one := []PathMap{m}
		for i, ptr := range out {
			out[i] = RewritePointer(one, ptr)
		}
	}

	return out
}

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
