package domain

import (
	"testing"
)

func TestResolveReturnsTheNodeAtAPath(t *testing.T) {
	t.Parallel()

	d := newDocument(t)

	tests := []struct {
		name string
		path Path
		want Node // nil means the path leads nowhere
	}{
		{
			name: "the root",
			path: Path{},
			want: d.root,
		},
		{
			name: "a member of the root",
			path: path(KeySegment("server")),
			want: d.server,
		},
		{
			name: "a nested member",
			path: path(KeySegment("server"), KeySegment("host")),
			want: d.host,
		},
		{
			name: "an array",
			path: path(KeySegment("server"), KeySegment("features")),
			want: d.features,
		},
		{
			name: "an array element",
			path: path(
				KeySegment("server"),
				KeySegment("features"),
				IndexSegment(1),
			),
			want: d.history,
		},
		{
			// The cursor holds an index segment and a parsed pointer holds a
			// key segment. Both name the same element.
			name: "an array element addressed by a key segment",
			path: path(
				KeySegment("server"),
				KeySegment("features"),
				KeySegment("0"),
			),
			want: d.search,
		},
		{
			name: "a key holding a solidus",
			path: path(KeySegment("a/b")),
			want: d.slash,
		},
		{
			// The member is named "0" and the container is an object, so the
			// token is a key however it reads.
			name: "a key that looks like an index",
			path: path(IndexSegment(0)),
			want: d.numeric,
		},
		{
			name: "a key the object does not have",
			path: path(KeySegment("client")),
			want: nil,
		},
		{
			name: "a key below a missing one",
			path: path(KeySegment("client"), KeySegment("host")),
			want: nil,
		},
		{
			name: "past the end of an array",
			path: path(
				KeySegment("server"),
				KeySegment("features"),
				IndexSegment(2),
			),
			want: nil,
		},
		{
			name: "a word where an array wants an index",
			path: path(
				KeySegment("server"),
				KeySegment("features"),
				KeySegment("first"),
			),
			want: nil,
		},
		{
			// RFC 6901 spells positions without a leading zero, so "01" names
			// nothing even though it reads as one.
			name: "an index spelled with a leading zero",
			path: path(
				KeySegment("server"),
				KeySegment("features"),
				KeySegment("01"),
			),
			want: nil,
		},
		{
			name: "an index spelled with a sign",
			path: path(
				KeySegment("server"),
				KeySegment("features"),
				KeySegment("+1"),
			),
			want: nil,
		},
		{
			name: "a negative index",
			path: path(
				KeySegment("server"),
				KeySegment("features"),
				KeySegment("-1"),
			),
			want: nil,
		},
		{
			name: "a step taken from a string",
			path: path(
				KeySegment("server"),
				KeySegment("host"),
				KeySegment("length"),
			),
			want: nil,
		},
		{
			name: "a step taken from a null",
			path: path(KeySegment("0"), KeySegment("anything")),
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := Resolve(d.root, tt.path)

			if tt.want == nil {
				if ok {
					t.Fatalf("Resolve(%q) succeeded, want no node", tt.path)
				}

				if got != nil {
					t.Errorf("Resolve(%q) = %v, want nil alongside false", tt.path, got)
				}

				return
			}

			if !ok {
				t.Fatalf("Resolve(%q) found nothing, want a node", tt.path)
			}

			// Comparing the pointers rather than the contents: an immutable
			// tree is identified by identity, and two nodes can look alike.
			if got != tt.want {
				t.Errorf("Resolve(%q) returned a different node of kind %s", tt.path, got.Kind())
			}
		})
	}
}

// A pointer yields key segments throughout, since text alone cannot say
// whether a step enters an array. Resolving one has to work anyway, which is
// what the future jump to a pointer typed by the user will rest on.
func TestResolveFollowsAParsedPointer(t *testing.T) {
	t.Parallel()

	d := newDocument(t)

	tests := []struct {
		pointer string
		want    Node
	}{
		{pointer: "", want: d.root},
		{pointer: "/server", want: d.server},
		{pointer: "/server/port", want: d.port},
		{pointer: "/server/features/1", want: d.history},
		{pointer: "/a~1b", want: d.slash},
		{pointer: "/0", want: d.numeric},
	}

	for _, tt := range tests {
		t.Run(tt.pointer, func(t *testing.T) {
			t.Parallel()

			p, err := ParsePointer(tt.pointer)
			if err != nil {
				t.Fatalf("ParsePointer(%q) error = %v", tt.pointer, err)
			}

			got, ok := Resolve(d.root, p)
			if !ok {
				t.Fatalf("Resolve(%q) found nothing", tt.pointer)
			}

			if got != tt.want {
				t.Errorf("Resolve(%q) returned a different node of kind %s", tt.pointer, got.Kind())
			}
		})
	}
}

// The status bar asks for the node under the cursor before anything is open.
func TestResolveFindsNothingWithoutADocument(t *testing.T) {
	t.Parallel()

	tests := map[string]Path{
		"the root":   {},
		"a key":      path(KeySegment("server")),
		"deeper yet": path(KeySegment("server"), IndexSegment(0)),
	}

	for name, p := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, ok := Resolve(nil, p)
			if ok {
				t.Errorf("Resolve(nil, %q) succeeded, want no node", p)
			}

			if got != nil {
				t.Errorf("Resolve(nil, %q) = %v, want nil", p, got)
			}
		})
	}
}

// A Node holding a nil pointer must be answered like an absent one rather than
// handed back as a node, or read into and panicked on.
func TestResolveTreatsATypedNilRootAsAbsent(t *testing.T) {
	t.Parallel()

	paths := map[string]Path{
		"the root": {},
		"a key":    path(KeySegment("server")),
		"an index": path(IndexSegment(0)),
	}

	for kind, root := range typedNils() {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			for name, p := range paths {
				t.Run(name, func(t *testing.T) {
					t.Parallel()

					got, ok := Resolve(root, p)
					if ok {
						t.Errorf("Resolve(%s(nil), %q) succeeded, want no node", kind, p)
					}

					if got != nil {
						t.Errorf("Resolve(%s(nil), %q) = %v, want nil", kind, p, got)
					}
				})
			}
		})
	}
}

// Every path the renderer builds has to resolve, since the cursor and the
// collapsed set are nothing but such paths.
func TestResolveFindsEveryNodeOfADocument(t *testing.T) {
	t.Parallel()

	d := newDocument(t)

	var walk func(n Node, p Path)
	walk = func(n Node, p Path) {
		got, ok := Resolve(d.root, p)
		if !ok {
			t.Errorf("Resolve(%q) found nothing", p)

			return
		}

		if got != n {
			t.Errorf("Resolve(%q) returned a different node", p)
		}

		switch v := n.(type) {
		case *Object:
			for _, m := range v.All() {
				walk(m.Value, p.Child(KeySegment(m.Key)))
			}

		case *Array:
			for i, e := range v.All() {
				walk(e, p.Child(IndexSegment(i)))
			}
		}
	}

	walk(d.root, Path{})
}
