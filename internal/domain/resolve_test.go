package domain_test

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// document is a tree with each of its nodes named, so that a test can assert
// which one came back rather than only what it looks like.
type document struct {
	root     domain.Node
	server   domain.Node
	host     domain.Node
	port     domain.Node
	features domain.Node
	search   domain.Node
	history  domain.Node
	slash    domain.Node
	numeric  domain.Node
}

func newDocument(t *testing.T) document {
	t.Helper()

	var d document

	d.host = str(t, "localhost")
	d.port = domain.NewNumber("8080")
	d.search = str(t, "search")
	d.history = str(t, "history")
	d.features = domain.NewArray([]domain.Node{d.search, d.history})

	d.server = obj(t,
		domain.Member{Key: "host", Value: d.host},
		domain.Member{Key: "port", Value: d.port},
		domain.Member{Key: "features", Value: d.features},
	)

	// A key holding a solidus and a key that reads as a number: both are legal
	// JSON, and both are where addressing a node by text goes wrong if the
	// token is not taken literally.
	d.slash = domain.NewBool(true)
	d.numeric = domain.NewNull()

	d.root = obj(t,
		domain.Member{Key: "server", Value: d.server},
		domain.Member{Key: "a/b", Value: d.slash},
		domain.Member{Key: "0", Value: d.numeric},
	)

	return d
}

func TestResolve(t *testing.T) {
	t.Parallel()

	d := newDocument(t)

	tests := []struct {
		name string
		path domain.Path
		want domain.Node // nil means the path leads nowhere
	}{
		{
			name: "the root",
			path: domain.Path{},
			want: d.root,
		},
		{
			name: "a member of the root",
			path: path(domain.KeySegment("server")),
			want: d.server,
		},
		{
			name: "a nested member",
			path: path(domain.KeySegment("server"), domain.KeySegment("host")),
			want: d.host,
		},
		{
			name: "an array",
			path: path(domain.KeySegment("server"), domain.KeySegment("features")),
			want: d.features,
		},
		{
			name: "an array element",
			path: path(
				domain.KeySegment("server"),
				domain.KeySegment("features"),
				domain.IndexSegment(1),
			),
			want: d.history,
		},
		{
			// The cursor holds an index segment and a parsed pointer holds a
			// key segment. Both name the same element.
			name: "an array element addressed by a key segment",
			path: path(
				domain.KeySegment("server"),
				domain.KeySegment("features"),
				domain.KeySegment("0"),
			),
			want: d.search,
		},
		{
			name: "a key holding a solidus",
			path: path(domain.KeySegment("a/b")),
			want: d.slash,
		},
		{
			// The member is named "0" and the container is an object, so the
			// token is a key however it reads.
			name: "a key that looks like an index",
			path: path(domain.IndexSegment(0)),
			want: d.numeric,
		},
		{
			name: "a key the object does not have",
			path: path(domain.KeySegment("client")),
			want: nil,
		},
		{
			name: "a key below a missing one",
			path: path(domain.KeySegment("client"), domain.KeySegment("host")),
			want: nil,
		},
		{
			name: "past the end of an array",
			path: path(
				domain.KeySegment("server"),
				domain.KeySegment("features"),
				domain.IndexSegment(2),
			),
			want: nil,
		},
		{
			name: "a word where an array wants an index",
			path: path(
				domain.KeySegment("server"),
				domain.KeySegment("features"),
				domain.KeySegment("first"),
			),
			want: nil,
		},
		{
			// RFC 6901 spells positions without a leading zero, so "01" names
			// nothing even though it reads as one.
			name: "an index spelled with a leading zero",
			path: path(
				domain.KeySegment("server"),
				domain.KeySegment("features"),
				domain.KeySegment("01"),
			),
			want: nil,
		},
		{
			name: "an index spelled with a sign",
			path: path(
				domain.KeySegment("server"),
				domain.KeySegment("features"),
				domain.KeySegment("+1"),
			),
			want: nil,
		},
		{
			name: "a negative index",
			path: path(
				domain.KeySegment("server"),
				domain.KeySegment("features"),
				domain.KeySegment("-1"),
			),
			want: nil,
		},
		{
			name: "a step taken from a string",
			path: path(
				domain.KeySegment("server"),
				domain.KeySegment("host"),
				domain.KeySegment("length"),
			),
			want: nil,
		},
		{
			name: "a step taken from a null",
			path: path(domain.KeySegment("0"), domain.KeySegment("anything")),
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := domain.Resolve(d.root, tt.path)

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
func TestResolveFromParsedPointer(t *testing.T) {
	t.Parallel()

	d := newDocument(t)

	tests := []struct {
		pointer string
		want    domain.Node
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

			p, err := domain.ParsePointer(tt.pointer)
			if err != nil {
				t.Fatalf("ParsePointer(%q) error = %v", tt.pointer, err)
			}

			got, ok := domain.Resolve(d.root, p)
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
func TestResolveWithoutDocument(t *testing.T) {
	t.Parallel()

	tests := map[string]domain.Path{
		"the root":   {},
		"a key":      path(domain.KeySegment("server")),
		"deeper yet": path(domain.KeySegment("server"), domain.IndexSegment(0)),
	}

	for name, p := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, ok := domain.Resolve(nil, p)
			if ok {
				t.Errorf("Resolve(nil, %q) succeeded, want no node", p)
			}

			if got != nil {
				t.Errorf("Resolve(nil, %q) = %v, want nil", p, got)
			}
		})
	}
}

// typedNils are Node values holding a nil pointer, one per kind. They do not
// compare equal to nil, so anything reading a field of one panics.
func typedNils() map[string]domain.Node {
	return map[string]domain.Node{
		"object": (*domain.Object)(nil),
		"array":  (*domain.Array)(nil),
		"string": (*domain.String)(nil),
		"number": (*domain.Number)(nil),
		"bool":   (*domain.Bool)(nil),
		"null":   (*domain.Null)(nil),
	}
}

// A Node holding a nil pointer must be answered like an absent one rather than
// handed back as a node, or read into and panicked on.
func TestResolveWithTypedNilRoot(t *testing.T) {
	t.Parallel()

	paths := map[string]domain.Path{
		"the root": {},
		"a key":    path(domain.KeySegment("server")),
		"an index": path(domain.IndexSegment(0)),
	}

	for kind, root := range typedNils() {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			for name, p := range paths {
				t.Run(name, func(t *testing.T) {
					t.Parallel()

					got, ok := domain.Resolve(root, p)
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

// A member with no value would leave a hole in the tree that panics in
// whichever walk reaches it first, far from where it was put there.
func TestNewObjectRejectsMissingValue(t *testing.T) {
	t.Parallel()

	t.Run("no value at all", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if recover() == nil {
				t.Error("NewObject returned normally, want a panic")
			}
		}()

		_, _ = domain.NewObject([]domain.Member{{Key: "host"}})
	})

	for kind, nilNode := range typedNils() {
		t.Run("a nil "+kind, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if recover() == nil {
					t.Errorf("NewObject with a nil %s returned normally, want a panic", kind)
				}
			}()

			_, _ = domain.NewObject([]domain.Member{{Key: "host", Value: nilNode}})
		})
	}
}

func TestNewArrayRejectsMissingElement(t *testing.T) {
	t.Parallel()

	t.Run("no value at all", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if recover() == nil {
				t.Error("NewArray returned normally, want a panic")
			}
		}()

		_ = domain.NewArray([]domain.Node{nil})
	})

	for kind, nilNode := range typedNils() {
		t.Run("a nil "+kind, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if recover() == nil {
					t.Errorf("NewArray with a nil %s returned normally, want a panic", kind)
				}
			}()

			// Not in first position, so that a check looking only at the head
			// of the slice would not pass.
			_ = domain.NewArray([]domain.Node{str(t, "search"), nilNode})
		})
	}
}

// Every path the renderer builds has to resolve, since the cursor and the
// collapsed set are nothing but such paths.
func TestResolveFindsEveryNodeOfADocument(t *testing.T) {
	t.Parallel()

	d := newDocument(t)

	var walk func(n domain.Node, p domain.Path)
	walk = func(n domain.Node, p domain.Path) {
		got, ok := domain.Resolve(d.root, p)
		if !ok {
			t.Errorf("Resolve(%q) found nothing", p)

			return
		}

		if got != n {
			t.Errorf("Resolve(%q) returned a different node", p)
		}

		switch v := n.(type) {
		case *domain.Object:
			for _, m := range v.All() {
				walk(m.Value, p.Child(domain.KeySegment(m.Key)))
			}

		case *domain.Array:
			for i, e := range v.All() {
				walk(e, p.Child(domain.IndexSegment(i)))
			}
		}
	}

	walk(d.root, domain.Path{})
}
