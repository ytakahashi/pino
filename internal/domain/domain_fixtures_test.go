package domain

import (
	"testing"
)

// str builds a String, failing the test if the value is not valid UTF-8.
func str(t *testing.T, v string) *String {
	t.Helper()

	s, err := NewString(v)
	if err != nil {
		t.Fatalf("NewString(%q): %v", v, err)
	}

	return s
}

// obj builds an Object, failing the test if the members are not admissible.
func obj(t *testing.T, members ...Member) *Object {
	t.Helper()

	o, err := NewObject(members)
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	return o
}

// comment builds a Comment, failing the test if its text breaks the chosen
// delimiter form.
func comment(t *testing.T, text string, block, ownLine bool) Comment {
	t.Helper()

	c, err := NewComment(text, block, ownLine)
	if err != nil {
		t.Fatalf("NewComment(%q): %v", text, err)
	}

	return c
}

// path builds a Path from segments, the way the renderer walks a tree.
func path(segs ...Segment) Path {
	p := Path{}
	for _, s := range segs {
		p = p.Child(s)
	}

	return p
}

// typedNils are Node values holding a nil pointer, one per kind. They do not
// compare equal to nil, so anything reading a field of one panics.
func typedNils() map[string]Node {
	return map[string]Node{
		"object": (*Object)(nil),
		"array":  (*Array)(nil),
		"string": (*String)(nil),
		"number": (*Number)(nil),
		"bool":   (*Bool)(nil),
		"null":   (*Null)(nil),
	}
}
