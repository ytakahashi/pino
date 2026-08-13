package domain

import (
	"errors"
	"math"
	"strconv"
	"testing"
)

func TestSegmentsExposeTheirKeyOrIndex(t *testing.T) {
	t.Parallel()

	key := KeySegment("host")
	if !key.IsKey() {
		t.Error("KeySegment().IsKey() = false")
	}

	if key.Key() != "host" {
		t.Errorf("Key() = %q, want %q", key.Key(), "host")
	}

	if key.Token() != "host" {
		t.Errorf("Token() = %q, want %q", key.Token(), "host")
	}

	idx := IndexSegment(12)
	if idx.IsKey() {
		t.Error("IndexSegment().IsKey() = true")
	}

	if idx.Index() != 12 {
		t.Errorf("Index() = %d, want 12", idx.Index())
	}

	if idx.Token() != "12" {
		t.Errorf("Token() = %q, want %q", idx.Token(), "12")
	}

	if KeySegment("host") != key {
		t.Error("segments built the same way did not compare equal")
	}
}

// A negative index has no meaning in JSON and would surface far from its
// origin: Array.At would panic during rendering, and Equal would treat it as
// the object key "-1".
func TestIndexSegmentRejectsNegativeIndex(t *testing.T) {
	t.Parallel()

	for _, i := range []int{-1, -42, math.MinInt} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			defer func() {
				if recover() == nil {
					t.Errorf("IndexSegment(%d) returned normally, want a panic", i)
				}
			}()

			_ = IndexSegment(i)
		})
	}
}

func TestIndexSegmentAcceptsZero(t *testing.T) {
	t.Parallel()

	seg := IndexSegment(0)

	if seg.IsKey() {
		t.Error("IndexSegment(0).IsKey() = true")
	}

	if seg.Index() != 0 {
		t.Errorf("Index() = %d, want 0", seg.Index())
	}

	if seg.Token() != "0" {
		t.Errorf("Token() = %q, want %q", seg.Token(), "0")
	}
}

func TestTheZeroPathIsTheRoot(t *testing.T) {
	t.Parallel()

	root := Path{}

	if !root.IsRoot() {
		t.Error("the zero Path is not the root")
	}

	if root.Len() != 0 {
		t.Errorf("Len() = %d, want 0", root.Len())
	}

	if got := root.String(); got != "" {
		t.Errorf("String() = %q, want %q (RFC 6901 spells the root empty)", got, "")
	}
}

func TestPathStringUsesJSONPointerSyntax(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path Path
		want string
	}{
		{
			name: "object member",
			path: path(KeySegment("server"), KeySegment("host")),
			want: "/server/host",
		},
		{
			name: "array element",
			path: path(KeySegment("features"), IndexSegment(1)),
			want: "/features/1",
		},
		{
			name: "empty key",
			path: path(KeySegment("")),
			want: "/",
		},
		{
			name: "key containing a slash",
			path: path(KeySegment("a/b")),
			want: "/a~1b",
		},
		{
			name: "key containing a tilde",
			path: path(KeySegment("m~n")),
			want: "/m~0n",
		},
		{
			// Escaping "~" first must not turn the "0" it produces into "~00".
			name: "key that already looks escaped",
			path: path(KeySegment("~1")),
			want: "/~01",
		},
		{
			name: "key with both specials",
			path: path(KeySegment("~/")),
			want: "/~0~1",
		},
		{
			name: "deep path",
			path: path(
				KeySegment("a"),
				IndexSegment(0),
				KeySegment("b"),
				IndexSegment(10),
			),
			want: "/a/0/b/10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.path.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParsePointerBuildsKeySegments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pointer string
		want    []string // expected tokens
	}{
		{"root", "", nil},
		{"single", "/host", []string{"host"}},
		{"nested", "/server/host", []string{"server", "host"}},
		{"numeric token", "/features/0", []string{"features", "0"}},
		{"empty key", "/", []string{""}},
		{"trailing empty key", "/a/", []string{"a", ""}},
		{"escaped slash", "/a~1b", []string{"a/b"}},
		{"escaped tilde", "/m~0n", []string{"m~n"}},
		{"escaped tilde then one", "/~01", []string{"~1"}},
		{"both specials", "/~0~1", []string{"~/"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParsePointer(tt.pointer)
			if err != nil {
				t.Fatalf("ParsePointer(%q) error = %v", tt.pointer, err)
			}

			if got.Len() != len(tt.want) {
				t.Fatalf("Len() = %d, want %d", got.Len(), len(tt.want))
			}

			for i, want := range tt.want {
				seg := got.At(i)
				if seg.Token() != want {
					t.Errorf("segment %d token = %q, want %q", i, seg.Token(), want)
				}

				if !seg.IsKey() {
					t.Errorf("segment %d is not a key segment; a pointer alone cannot say otherwise", i)
				}
			}
		})
	}
}

// Text going through ParsePointer and back must come out unchanged, escapes
// included.
func TestPointersSurviveAStringRoundTrip(t *testing.T) {
	t.Parallel()

	pointers := []string{
		"",
		"/host",
		"/server/host",
		"/features/0",
		"/",
		"/a/",
		"/a~1b",
		"/m~0n",
		"/~01",
		"/~0~1",
		"/a/0/b/10",
	}

	for _, pointer := range pointers {
		t.Run(pointer, func(t *testing.T) {
			t.Parallel()

			p, err := ParsePointer(pointer)
			if err != nil {
				t.Fatalf("ParsePointer(%q) error = %v", pointer, err)
			}

			if got := p.String(); got != pointer {
				t.Errorf("round trip = %q, want %q", got, pointer)
			}
		})
	}
}

func TestParsePointerRejectsMalformed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pointer string
	}{
		{"missing leading slash", "server/host"},
		{"bare word", "host"},
		{"tilde followed by nothing", "/a~"},
		{"tilde followed by a letter", "/a~b"},
		{"tilde followed by two", "/a~2"},
		{"tilde at the very end of a later token", "/a/b~"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParsePointer(tt.pointer)
			if err == nil {
				t.Fatalf("ParsePointer(%q) succeeded, want an error", tt.pointer)
			}

			var invalid *InvalidPointerError
			if !errors.As(err, &invalid) {
				t.Fatalf("error = %v, want *InvalidPointerError", err)
			}

			if invalid.Pointer != tt.pointer {
				t.Errorf("Pointer = %q, want %q", invalid.Pointer, tt.pointer)
			}

			if invalid.Reason == "" {
				t.Error("Reason is empty")
			}
		})
	}
}

// Child must never write into the parent's storage. Two children built from
// the same parent are the case that would break, and paths are handed to the
// renderer and stored in undo history, so a corrupted one is hard to trace
// back to its source.
func TestChildDoesNotDisturbTheParent(t *testing.T) {
	t.Parallel()

	parent := path(KeySegment("server"))

	host := parent.Child(KeySegment("host"))
	port := parent.Child(KeySegment("port"))

	if got := parent.String(); got != "/server" {
		t.Errorf("parent = %q, want %q", got, "/server")
	}

	if got := host.String(); got != "/server/host" {
		t.Errorf("first child = %q, want %q", got, "/server/host")
	}

	if got := port.String(); got != "/server/port" {
		t.Errorf("second child = %q, want %q", got, "/server/port")
	}

	// Deeper, where a shared backing array would have more room to bite.
	deep := Path{}
	for _, key := range []string{"a", "b", "c", "d"} {
		deep = deep.Child(KeySegment(key))
	}

	left := deep.Child(IndexSegment(0))
	right := deep.Child(IndexSegment(1))

	if got := left.String(); got != "/a/b/c/d/0" {
		t.Errorf("left = %q, want %q", got, "/a/b/c/d/0")
	}

	if got := right.String(); got != "/a/b/c/d/1" {
		t.Errorf("right = %q, want %q", got, "/a/b/c/d/1")
	}

	if got := deep.String(); got != "/a/b/c/d" {
		t.Errorf("deep = %q, want %q", got, "/a/b/c/d")
	}
}

func TestPathParentRemovesTheLastSegment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path Path
		want string
	}{
		{
			name: "deep path",
			path: path(KeySegment("a"), KeySegment("b"), KeySegment("c")),
			want: "/a/b",
		},
		{
			name: "array element",
			path: path(KeySegment("features"), IndexSegment(1)),
			want: "/features",
		},
		{
			name: "one level down",
			path: path(KeySegment("server")),
			want: "",
		},
		{
			// The root being its own parent is what lets a walk upwards stop on
			// IsRoot instead of on a second return value.
			name: "the root",
			path: Path{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.path.Parent().String(); got != tt.want {
				t.Errorf("Parent() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Climbing from any path has to reach the root, which is how a lost cursor is
// recovered: the walk stops because Parent stops shortening.
func TestPathParentClimbsToTheRoot(t *testing.T) {
	t.Parallel()

	p := path(
		KeySegment("a"),
		IndexSegment(0),
		KeySegment("b"),
		KeySegment("c"),
	)

	steps := 0
	for !p.IsRoot() {
		p = p.Parent()

		steps++
		if steps > 10 {
			t.Fatal("Parent() did not reach the root")
		}
	}

	if steps != 4 {
		t.Errorf("reached the root in %d steps, want 4", steps)
	}
}

// Parent shares its storage with the path it came from, so anything built
// from it must still not disturb its origin or its siblings. This is the same
// trap Child avoids, reached from the other direction.
func TestParentDoesNotDisturbTheOriginalPath(t *testing.T) {
	t.Parallel()

	original := path(
		KeySegment("server"),
		KeySegment("features"),
		IndexSegment(0),
	)

	parent := original.Parent()

	first := parent.Child(IndexSegment(7))
	second := parent.Child(IndexSegment(8))

	if got := original.String(); got != "/server/features/0" {
		t.Errorf("original = %q, want %q", got, "/server/features/0")
	}

	if got := parent.String(); got != "/server/features" {
		t.Errorf("parent = %q, want %q", got, "/server/features")
	}

	if got := first.String(); got != "/server/features/7" {
		t.Errorf("first child of the parent = %q, want %q", got, "/server/features/7")
	}

	if got := second.String(); got != "/server/features/8" {
		t.Errorf("second child of the parent = %q, want %q", got, "/server/features/8")
	}
}

func TestPathEqualComparesTokensAcrossSegmentTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b Path
		want bool
	}{
		{
			name: "both root",
			a:    Path{},
			b:    Path{},
			want: true,
		},
		{
			name: "same keys",
			a:    path(KeySegment("server"), KeySegment("host")),
			b:    path(KeySegment("server"), KeySegment("host")),
			want: true,
		},
		{
			name: "different keys",
			a:    path(KeySegment("server"), KeySegment("host")),
			b:    path(KeySegment("server"), KeySegment("port")),
			want: false,
		},
		{
			name: "different depth",
			a:    path(KeySegment("server")),
			b:    path(KeySegment("server"), KeySegment("host")),
			want: false,
		},
		{
			// A parsed pointer yields key segments, while the cursor holds an
			// index segment. Both name the same node.
			name: "index segment against the key segment parsed from it",
			a:    path(KeySegment("features"), IndexSegment(0)),
			b:    path(KeySegment("features"), KeySegment("0")),
			want: true,
		},
		{
			name: "index segments differing",
			a:    path(IndexSegment(1)),
			b:    path(IndexSegment(2)),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("a.Equal(b) = %v, want %v", got, tt.want)
			}

			if got := tt.b.Equal(tt.a); got != tt.want {
				t.Errorf("b.Equal(a) = %v, want %v (Equal must be symmetric)", got, tt.want)
			}
		})
	}
}

// A path parsed from what String produced must be equal to the original, even
// when the original mixes keys and indexes.
func TestParsedPointerEqualsTheOriginalPath(t *testing.T) {
	t.Parallel()

	original := path(
		KeySegment("server"),
		KeySegment("features"),
		IndexSegment(2),
		KeySegment("a/b"),
	)

	parsed, err := ParsePointer(original.String())
	if err != nil {
		t.Fatalf("ParsePointer(%q) error = %v", original.String(), err)
	}

	if !original.Equal(parsed) {
		t.Errorf("%q did not survive a round trip through its pointer", original.String())
	}
}

func TestPathAllYieldsEverySegmentInOrder(t *testing.T) {
	t.Parallel()

	p := path(KeySegment("server"), IndexSegment(3), KeySegment("host"))

	var got []string
	for i, s := range p.All() {
		if p.At(i) != s {
			t.Errorf("All() yielded %v at %d, but At(%d) = %v", s, i, i, p.At(i))
		}

		got = append(got, s.Token())
	}

	want := []string{"server", "3", "host"}
	if len(got) != len(want) {
		t.Fatalf("All() yielded %d segments, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("All()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
