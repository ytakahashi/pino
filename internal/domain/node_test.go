package domain

import (
	"errors"
	"testing"
)

func TestKindStringNamesEveryKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind Kind
		want string
	}{
		{KindObject, "object"},
		{KindArray, "array"},
		{KindString, "string"},
		{KindNumber, "number"},
		{KindBool, "boolean"},
		{KindNull, "null"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()

			if got := tt.kind.String(); got != tt.want {
				t.Errorf("Kind.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEveryNodeReportsItsKind(t *testing.T) {
	t.Parallel()

	obj, err := NewObject(nil)
	if err != nil {
		t.Fatalf("NewObject() error = %v", err)
	}

	tests := []struct {
		name string
		node Node
		want Kind
	}{
		{"object", obj, KindObject},
		{"array", NewArray(nil), KindArray},
		{"string", str(t, "x"), KindString},
		{"number", NewNumber("1"), KindNumber},
		{"bool", NewBool(true), KindBool},
		{"null", NewNull(), KindNull},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.node.Kind(); got != tt.want {
				t.Errorf("Kind() = %v, want %v", got, tt.want)
			}

			if !tt.node.Trivia().IsEmpty() {
				t.Error("Trivia() is not empty for a freshly built node")
			}
		})
	}
}

// Nodes must be comparable with ==, because IsDirty compares the current root
// against the saved one and the renderer recognises unchanged subtrees by
// pointer identity. A value type holding a slice would panic here.
func TestNodesAreComparable(t *testing.T) {
	t.Parallel()

	obj, err := NewObject([]Member{{Key: "a", Value: NewNull()}})
	if err != nil {
		t.Fatalf("NewObject() error = %v", err)
	}

	var (
		same  Node = obj
		other Node = NewArray([]Node{str(t, "x")})
	)

	if same != Node(obj) {
		t.Error("the same node did not compare equal to itself")
	}

	if obj == other {
		t.Error("distinct nodes compared equal")
	}
}

func TestNewObjectPreservesOrder(t *testing.T) {
	t.Parallel()

	members := []Member{
		{Key: "zebra", Value: NewNumber("1")},
		{Key: "apple", Value: NewNumber("2")},
		{Key: "mango", Value: NewNumber("3")},
	}

	obj, err := NewObject(members)
	if err != nil {
		t.Fatalf("NewObject() error = %v", err)
	}

	want := []string{"zebra", "apple", "mango"}

	if obj.Len() != len(want) {
		t.Fatalf("Len() = %d, want %d", obj.Len(), len(want))
	}

	for i, key := range want {
		if got := obj.At(i).Key; got != key {
			t.Errorf("At(%d).Key = %q, want %q", i, got, key)
		}
	}

	var got []string
	for _, m := range obj.All() {
		got = append(got, m.Key)
	}

	if len(got) != len(want) {
		t.Fatalf("All() yielded %d members, want %d", len(got), len(want))
	}

	for i, key := range want {
		if got[i] != key {
			t.Errorf("All()[%d] = %q, want %q", i, got[i], key)
		}
	}
}

func TestNewObjectRejectsDuplicateKey(t *testing.T) {
	t.Parallel()

	members := []Member{
		{Key: "port", Value: NewNumber("8080")},
		{Key: "host", Value: str(t, "localhost")},
		{Key: "port", Value: NewNumber("9090")},
	}

	_, err := NewObject(members)
	if err == nil {
		t.Fatal("NewObject() succeeded, want a duplicate key error")
	}

	var dup *DuplicateKeyError
	if !errors.As(err, &dup) {
		t.Fatalf("NewObject() error = %v, want *DuplicateKeyError", err)
	}

	if dup.Key != "port" {
		t.Errorf("Key = %q, want %q", dup.Key, "port")
	}

	if dup.First != 0 {
		t.Errorf("First = %d, want 0", dup.First)
	}

	if dup.Dup != 2 {
		t.Errorf("Dup = %d, want 2", dup.Dup)
	}
}

// RFC 8259 requires JSON to be UTF-8. A document is refused where the bytes
// would enter the tree, so that nothing downstream has to decide what to do
// with text it cannot encode.
func TestNewStringRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in    string
		index int
	}{
		"a lone continuation byte": {in: "\xff", index: 0},
		"a truncated sequence":     {in: "\xe8\xa8", index: 0},
		"an overlong encoding":     {in: "\xc0\xaf", index: 0},
		"an encoded surrogate":     {in: "\xed\xa0\x80", index: 0},
		"a bad byte after text":    {in: "ok\xffbad", index: 2},
		"a bad byte after kanji":   {in: "設定\xff", index: 6},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := NewString(tc.in)
			if err == nil {
				t.Fatalf("NewString(%q) succeeded, want an invalid UTF-8 error", tc.in)
			}

			var bad *InvalidUTF8Error
			if !errors.As(err, &bad) {
				t.Fatalf("NewString(%q) error = %v, want *InvalidUTF8Error", tc.in, err)
			}

			if bad.Index != tc.index {
				t.Errorf("Index = %d, want %d", bad.Index, tc.index)
			}

			if bad.Text != tc.in {
				t.Errorf("Text = %q, want %q", bad.Text, tc.in)
			}
		})
	}
}

func TestNewStringAcceptsValidUTF8(t *testing.T) {
	t.Parallel()

	// U+FFFD is a character like any other: text that already contains one
	// must not be mistaken for text that failed to decode.
	for _, in := range []string{"", "localhost", "設定", "🌲", "�", "a\x00b"} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			s, err := NewString(in)
			if err != nil {
				t.Fatalf("NewString(%q) error = %v", in, err)
			}

			if s.Value() != in {
				t.Errorf("Value() = %q, want %q", s.Value(), in)
			}
		})
	}
}

// A key is a JSON string as much as a value is, and it reaches a document
// through Member rather than through a constructor of its own.
func TestNewObjectRejectsInvalidKey(t *testing.T) {
	t.Parallel()

	_, err := NewObject([]Member{
		{Key: "host", Value: str(t, "localhost")},
		{Key: "po\xffrt", Value: NewNumber("8080")},
	})
	if err == nil {
		t.Fatal("NewObject() succeeded, want an invalid UTF-8 error")
	}

	var bad *InvalidUTF8Error
	if !errors.As(err, &bad) {
		t.Fatalf("NewObject() error = %v, want *InvalidUTF8Error", err)
	}

	if bad.Text != "po\xffrt" || bad.Index != 2 {
		t.Errorf("error = %+v, want the offending key at byte 2", bad)
	}
}

func TestObjectFindsMembersByKey(t *testing.T) {
	t.Parallel()

	obj, err := NewObject([]Member{
		{Key: "host", Value: str(t, "localhost")},
		{Key: "port", Value: NewNumber("8080")},
	})
	if err != nil {
		t.Fatalf("NewObject() error = %v", err)
	}

	m, ok := obj.Lookup("port")
	if !ok {
		t.Fatal("Lookup(\"port\") not found")
	}

	num, ok := m.Value.(*Number)
	if !ok {
		t.Fatalf("member value is %T, want *Number", m.Value)
	}

	if num.Raw() != "8080" {
		t.Errorf("Raw() = %q, want %q", num.Raw(), "8080")
	}

	if i, ok := obj.IndexOf("port"); !ok || i != 1 {
		t.Errorf("IndexOf(\"port\") = (%d, %v), want (1, true)", i, ok)
	}

	if _, ok := obj.Lookup("missing"); ok {
		t.Error("Lookup found a key that was never added")
	}

	if _, ok := obj.IndexOf("missing"); ok {
		t.Error("IndexOf found a key that was never added")
	}
}

// The constructors copy their input so that a caller reusing the slice cannot
// reach into a built node. Without this the index would drift out of step with
// the members, and shared subtrees would change under an undo.
func TestConstructorsCopyTheirInput(t *testing.T) {
	t.Parallel()

	members := []Member{{Key: "host", Value: str(t, "localhost")}}

	obj, err := NewObject(members)
	if err != nil {
		t.Fatalf("NewObject() error = %v", err)
	}

	members[0] = Member{Key: "hijacked", Value: NewNull()}

	if got := obj.At(0).Key; got != "host" {
		t.Errorf("At(0).Key = %q, want %q: the members slice was not copied", got, "host")
	}

	if _, ok := obj.Lookup("host"); !ok {
		t.Error("Lookup(\"host\") failed after the caller mutated its slice")
	}

	elements := []Node{str(t, "search")}

	arr := NewArray(elements)
	elements[0] = str(t, "hijacked")

	first, ok := arr.At(0).(*String)
	if !ok {
		t.Fatalf("element is %T, want *String", arr.At(0))
	}

	if first.Value() != "search" {
		t.Errorf("At(0) = %q, want %q: the elements slice was not copied", first.Value(), "search")
	}
}

// The comments carried by a member must survive the caller reusing the slice
// it built them from.
func TestObjectKeepsTriviaOutOfReach(t *testing.T) {
	t.Parallel()

	comments := []Comment{{Text: "the listening port"}}

	members := []Member{{
		Key:    "port",
		Value:  NewNumber("8080"),
		Trivia: NewTrivia(comments, nil),
	}}

	obj, err := NewObject(members)
	if err != nil {
		t.Fatalf("NewObject() error = %v", err)
	}

	comments[0].Text = "hijacked"

	for c := range obj.At(0).Trivia.Before() {
		if c.Text != "the listening port" {
			t.Errorf("comment = %q, want %q", c.Text, "the listening port")
		}
	}
}

// A Member is handed out by value, so writing to the copy must not reach the
// object it came from.
func TestMemberIsHandedOutByValue(t *testing.T) {
	t.Parallel()

	obj, err := NewObject([]Member{{Key: "host", Value: str(t, "localhost")}})
	if err != nil {
		t.Fatalf("NewObject() error = %v", err)
	}

	m := obj.At(0)
	m.Key = "hijacked"
	m.Value = NewNull()

	if got := obj.At(0).Key; got != "host" {
		t.Errorf("At(0).Key = %q, want %q", got, "host")
	}

	if obj.At(0).Value.Kind() != KindString {
		t.Error("the member value changed through a copy")
	}
}

func TestArrayKeepsElementsInOrder(t *testing.T) {
	t.Parallel()

	arr := NewArray([]Node{
		str(t, "search"),
		str(t, "history"),
	})

	if arr.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", arr.Len())
	}

	var got []string

	for _, n := range arr.All() {
		s, ok := n.(*String)
		if !ok {
			t.Fatalf("element is %T, want *String", n)
		}

		got = append(got, s.Value())
	}

	want := []string{"search", "history"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("All()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNumberKeepsSourceText(t *testing.T) {
	t.Parallel()

	// Round-tripping through float64 would rewrite every one of these.
	for _, raw := range []string{"8080", "0.1", "1e400", "1.0", "-0", "12345678901234567890"} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			if got := NewNumber(raw).Raw(); got != raw {
				t.Errorf("Raw() = %q, want %q", got, raw)
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

		_, _ = NewObject([]Member{{Key: "host"}})
	})

	for kind, nilNode := range typedNils() {
		t.Run("a nil "+kind, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if recover() == nil {
					t.Errorf("NewObject with a nil %s returned normally, want a panic", kind)
				}
			}()

			_, _ = NewObject([]Member{{Key: "host", Value: nilNode}})
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

		_ = NewArray([]Node{nil})
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
			_ = NewArray([]Node{str(t, "search"), nilNode})
		})
	}
}
