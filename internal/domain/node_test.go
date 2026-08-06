package domain_test

import (
	"errors"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

func TestKindString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind domain.Kind
		want string
	}{
		{domain.KindObject, "object"},
		{domain.KindArray, "array"},
		{domain.KindString, "string"},
		{domain.KindNumber, "number"},
		{domain.KindBool, "boolean"},
		{domain.KindNull, "null"},
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

func TestNodeKinds(t *testing.T) {
	t.Parallel()

	obj, err := domain.NewObject(nil)
	if err != nil {
		t.Fatalf("NewObject() error = %v", err)
	}

	tests := []struct {
		name string
		node domain.Node
		want domain.Kind
	}{
		{"object", obj, domain.KindObject},
		{"array", domain.NewArray(nil), domain.KindArray},
		{"string", domain.NewString("x"), domain.KindString},
		{"number", domain.NewNumber("1"), domain.KindNumber},
		{"bool", domain.NewBool(true), domain.KindBool},
		{"null", domain.NewNull(), domain.KindNull},
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

	obj, err := domain.NewObject([]domain.Member{{Key: "a", Value: domain.NewNull()}})
	if err != nil {
		t.Fatalf("NewObject() error = %v", err)
	}

	var (
		same  domain.Node = obj
		other domain.Node = domain.NewArray([]domain.Node{domain.NewString("x")})
	)

	if same != domain.Node(obj) {
		t.Error("the same node did not compare equal to itself")
	}

	if obj == other {
		t.Error("distinct nodes compared equal")
	}
}

func TestNewObjectPreservesOrder(t *testing.T) {
	t.Parallel()

	members := []domain.Member{
		{Key: "zebra", Value: domain.NewNumber("1")},
		{Key: "apple", Value: domain.NewNumber("2")},
		{Key: "mango", Value: domain.NewNumber("3")},
	}

	obj, err := domain.NewObject(members)
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

	members := []domain.Member{
		{Key: "port", Value: domain.NewNumber("8080")},
		{Key: "host", Value: domain.NewString("localhost")},
		{Key: "port", Value: domain.NewNumber("9090")},
	}

	_, err := domain.NewObject(members)
	if err == nil {
		t.Fatal("NewObject() succeeded, want a duplicate key error")
	}

	var dup *domain.DuplicateKeyError
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

func TestObjectLookup(t *testing.T) {
	t.Parallel()

	obj, err := domain.NewObject([]domain.Member{
		{Key: "host", Value: domain.NewString("localhost")},
		{Key: "port", Value: domain.NewNumber("8080")},
	})
	if err != nil {
		t.Fatalf("NewObject() error = %v", err)
	}

	m, ok := obj.Lookup("port")
	if !ok {
		t.Fatal("Lookup(\"port\") not found")
	}

	num, ok := m.Value.(*domain.Number)
	if !ok {
		t.Fatalf("member value is %T, want *domain.Number", m.Value)
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

	members := []domain.Member{{Key: "host", Value: domain.NewString("localhost")}}

	obj, err := domain.NewObject(members)
	if err != nil {
		t.Fatalf("NewObject() error = %v", err)
	}

	members[0] = domain.Member{Key: "hijacked", Value: domain.NewNull()}

	if got := obj.At(0).Key; got != "host" {
		t.Errorf("At(0).Key = %q, want %q: the members slice was not copied", got, "host")
	}

	if _, ok := obj.Lookup("host"); !ok {
		t.Error("Lookup(\"host\") failed after the caller mutated its slice")
	}

	elements := []domain.Node{domain.NewString("search")}

	arr := domain.NewArray(elements)
	elements[0] = domain.NewString("hijacked")

	first, ok := arr.At(0).(*domain.String)
	if !ok {
		t.Fatalf("element is %T, want *domain.String", arr.At(0))
	}

	if first.Value() != "search" {
		t.Errorf("At(0) = %q, want %q: the elements slice was not copied", first.Value(), "search")
	}
}

// Trivia travels inside Member, which is handed out by value. Copying the
// Member copies the slice headers but not their backing arrays, so Trivia has
// to be immutable in its own right for a handed-out Member to be harmless.
func TestNewTriviaCopiesItsInput(t *testing.T) {
	t.Parallel()

	before := []domain.Comment{{Text: "above"}}
	after := []domain.Comment{{Text: "trailing", Block: true}}

	tv := domain.NewTrivia(before, after)

	before[0].Text = "hijacked"
	after[0].Text = "hijacked"

	var gotBefore []domain.Comment
	for c := range tv.Before() {
		gotBefore = append(gotBefore, c)
	}

	if len(gotBefore) != 1 || gotBefore[0].Text != "above" {
		t.Errorf("Before() = %+v, want one comment %q", gotBefore, "above")
	}

	var gotAfter []domain.Comment
	for c := range tv.After() {
		gotAfter = append(gotAfter, c)
	}

	if len(gotAfter) != 1 || gotAfter[0].Text != "trailing" || !gotAfter[0].Block {
		t.Errorf("After() = %+v, want one block comment %q", gotAfter, "trailing")
	}

	if tv.IsEmpty() {
		t.Error("IsEmpty() = true for a Trivia holding comments")
	}

	if !domain.NewTrivia(nil, nil).IsEmpty() {
		t.Error("IsEmpty() = false for a Trivia with no comments")
	}
}

// The comments carried by a member must survive the caller reusing the slice
// it built them from.
func TestObjectKeepsTriviaOutOfReach(t *testing.T) {
	t.Parallel()

	comments := []domain.Comment{{Text: "the listening port"}}

	members := []domain.Member{{
		Key:    "port",
		Value:  domain.NewNumber("8080"),
		Trivia: domain.NewTrivia(comments, nil),
	}}

	obj, err := domain.NewObject(members)
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

	obj, err := domain.NewObject([]domain.Member{{Key: "host", Value: domain.NewString("localhost")}})
	if err != nil {
		t.Fatalf("NewObject() error = %v", err)
	}

	m := obj.At(0)
	m.Key = "hijacked"
	m.Value = domain.NewNull()

	if got := obj.At(0).Key; got != "host" {
		t.Errorf("At(0).Key = %q, want %q", got, "host")
	}

	if obj.At(0).Value.Kind() != domain.KindString {
		t.Error("the member value changed through a copy")
	}
}

func TestArray(t *testing.T) {
	t.Parallel()

	arr := domain.NewArray([]domain.Node{
		domain.NewString("search"),
		domain.NewString("history"),
	})

	if arr.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", arr.Len())
	}

	var got []string

	for _, n := range arr.All() {
		s, ok := n.(*domain.String)
		if !ok {
			t.Fatalf("element is %T, want *domain.String", n)
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

			if got := domain.NewNumber(raw).Raw(); got != raw {
				t.Errorf("Raw() = %q, want %q", got, raw)
			}
		})
	}
}
