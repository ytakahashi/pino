package domain_test

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// str builds a String, failing the test if the value is not valid UTF-8.
func str(t *testing.T, v string) *domain.String {
	t.Helper()

	s, err := domain.NewString(v)
	if err != nil {
		t.Fatalf("NewString(%q): %v", v, err)
	}

	return s
}

// obj builds an Object, failing the test if the members are not admissible.
func obj(t *testing.T, members ...domain.Member) *domain.Object {
	t.Helper()

	o, err := domain.NewObject(members)
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	return o
}

// path builds a Path from segments, the way the renderer walks a tree.
func path(segs ...domain.Segment) domain.Path {
	p := domain.Path{}
	for _, s := range segs {
		p = p.Child(s)
	}

	return p
}
