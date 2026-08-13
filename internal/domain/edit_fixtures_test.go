package domain_test

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
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
	root     *domain.Object
	server   *domain.Object
	host     *domain.String
	port     *domain.Number
	features *domain.Array
	first    *domain.Object
	second   *domain.Object
	third    *domain.Object
	debug    *domain.Bool
	empty    *domain.Object
}

func newTree(t *testing.T) tree {
	t.Helper()

	var d tree

	d.host = str(t, "localhost")
	d.port = domain.NewNumber("8080")
	d.server = obj(t,
		domain.Member{Key: "host", Value: d.host},
		domain.Member{Key: "port", Value: d.port},
	)

	// The elements hold a child each, so that a rename of an element can be
	// checked to carry what is underneath it.
	d.first = obj(t, domain.Member{Key: "name", Value: str(t, "first")})
	d.second = obj(t, domain.Member{Key: "name", Value: str(t, "second")})
	d.third = obj(t, domain.Member{Key: "name", Value: str(t, "third")})
	d.features = domain.NewArray([]domain.Node{d.first, d.second, d.third})

	d.debug = domain.NewBool(false)
	d.empty = obj(t)

	d.root = obj(t,
		domain.Member{Key: "server", Value: d.server},
		domain.Member{Key: "features", Value: d.features},
		domain.Member{Key: "debug", Value: d.debug},
		domain.Member{Key: "empty", Value: d.empty},
	)

	return d
}

// at is the path a JSON Pointer names, for a test that wants to read as the
// document does.
func at(t *testing.T, pointer string) domain.Path {
	t.Helper()

	p, err := domain.ParsePointer(pointer)
	if err != nil {
		t.Fatalf("ParsePointer(%q): %v", pointer, err)
	}

	return p
}

// nodeAt is the node a pointer names, failing the test when the path leads
// nowhere.
func nodeAt(t *testing.T, root domain.Node, pointer string) domain.Node {
	t.Helper()

	n, ok := domain.Resolve(root, at(t, pointer))
	if !ok {
		t.Fatalf("%q does not resolve", pointer)
	}

	return n
}

// childCount is how many children a container holds directly.
func childCount(t *testing.T, n domain.Node) int {
	t.Helper()

	switch n.Kind() {
	case domain.KindObject:
		return n.(*domain.Object).Len()
	case domain.KindArray:
		return n.(*domain.Array).Len()
	case domain.KindString, domain.KindNumber, domain.KindBool, domain.KindNull:
		t.Fatalf("childCount: %v holds no children", n.Kind())

		return 0
	default:
		t.Fatalf("childCount: %v holds no children", n.Kind())

		return 0
	}
}

// pairs is the renames written as pointers, which is how a test spells what it
// expects without building paths by hand.
func pairs(maps []domain.PathMap) [][2]string {
	out := make([][2]string, 0, len(maps))
	for _, m := range maps {
		out = append(out, [2]string{m.From.String(), m.To.String()})
	}

	return out
}

// dropRemoved drops the pointers that named something the edit took out,
// which is the step that has to come before the renames are applied.
func dropRemoved(removed []domain.Path, pointers []string) []string {
	out := make([]string, 0, len(pointers))

	for _, ptr := range pointers {
		if !domain.PointerRemoved(removed, ptr) {
			out = append(out, ptr)
		}
	}

	return out
}

// paths is the removed paths written as pointers.
func paths(ps []domain.Path) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.String())
	}

	return out
}

// rewriteTogether moves pointers the way the collapsed set will: every pointer
// is looked at once, against the whole set of maps.
func rewriteTogether(maps []domain.PathMap, pointers []string) []string {
	out := make([]string, len(pointers))
	for i, ptr := range pointers {
		out[i] = domain.RewritePointer(maps, ptr)
	}

	return out
}

// rewriteInTurn moves pointers by applying each map to the whole set before
// looking at the next one. It exists so that a test can show this is not the
// same thing as rewriteTogether.
func rewriteInTurn(maps []domain.PathMap, pointers []string) []string {
	out := append([]string(nil), pointers...)

	for _, m := range maps {
		one := []domain.PathMap{m}
		for i, ptr := range out {
			out[i] = domain.RewritePointer(one, ptr)
		}
	}

	return out
}
