package domain

import (
	"testing"
)

func TestConvertCoversEveryPairOfKinds(t *testing.T) {
	t.Parallel()

	sources := []struct {
		name string
		node Node
	}{
		{name: "string", node: str(t, "abc")},
		{name: "number", node: NewNumber("8080")},
		{name: "boolean", node: NewBool(true)},
		{name: "null", node: NewNull()},
		{
			name: "object",
			node: obj(t, Member{Key: "a", Value: NewNumber("1")}),
		},
		{name: "array", node: NewArray([]Node{NewNumber("1")})},
	}

	targets := []struct {
		name string
		kind Kind
	}{
		{name: "string", kind: KindString},
		{name: "number", kind: KindNumber},
		{name: "boolean", kind: KindBool},
		{name: "null", kind: KindNull},
		{name: "object", kind: KindObject},
		{name: "array", kind: KindArray},
	}

	// One row per source, one column per target. Where a pair carries nothing
	// over, the zero value of the target kind is what comes back.
	want := map[string][]string{
		//          string    number  boolean  null    object  array
		"string":  {`"abc"`, "0", "false", "null", "{}", "[]"},
		"number":  {`"8080"`, "8080", "false", "null", "{}", "[]"},
		"boolean": {`"true"`, "0", "true", "null", "{}", "[]"},
		"null":    {`""`, "0", "false", "null", "{}", "[]"},
		"object":  {`""`, "0", "false", "null", "{...}", "[]"},
		"array":   {`""`, "0", "false", "null", "{}", "[...]"},
	}

	for _, src := range sources {
		for i, dst := range targets {
			t.Run(src.name+" to "+dst.name, func(t *testing.T) {
				t.Parallel()

				got, err := Convert(src.node, dst.kind)
				if err != nil {
					t.Fatalf("Convert(%s, %s): %v", src.name, dst.name, err)
				}

				if got.Kind() != dst.kind {
					t.Fatalf("Kind() = %v, want %v", got.Kind(), dst.kind)
				}

				if d := describe(t, got); d != want[src.name][i] {
					t.Errorf("Convert(%s, %s) = %s, want %s",
						src.name, dst.name, d, want[src.name][i])
				}
			})
		}
	}
}

func TestConvertCarriesAValueOverWhereTheTextIsTheSame(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		from Node
		kind Kind
		want string
	}{
		{
			name: "a string spelling a number",
			from: str(t, "8080"),
			kind: KindNumber,
			want: "8080",
		},
		{
			name: "a string spelling no number",
			from: str(t, "abc"),
			kind: KindNumber,
			want: "0",
		},
		{
			// The literal survives, so a value entered as 1.50 is still 1.50
			// after a trip through string.
			name: "a number with trailing zeros",
			from: NewNumber("1.50"),
			kind: KindString,
			want: `"1.50"`,
		},
		{
			name: "a number in exponent notation",
			from: NewNumber("1E+10"),
			kind: KindString,
			want: `"1E+10"`,
		},
		{
			name: "true seen as text",
			from: NewBool(true),
			kind: KindString,
			want: `"true"`,
		},
		{
			name: "false seen as text",
			from: NewBool(false),
			kind: KindString,
			want: `"false"`,
		},
		{
			name: "the text true",
			from: str(t, "true"),
			kind: KindBool,
			want: "true",
		},
		{
			name: "the text false",
			from: str(t, "false"),
			kind: KindBool,
			want: "false",
		},
		{
			name: "text that is neither",
			from: str(t, "yes"),
			kind: KindBool,
			want: "false",
		},
		{
			// JSON draws no correspondence between the two, so the number does
			// not come back as 1.
			name: "a boolean seen as a number",
			from: NewBool(true),
			kind: KindNumber,
			want: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Convert(tt.from, tt.kind)
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}

			if d := describe(t, got); d != tt.want {
				t.Errorf("Convert = %s, want %s", d, tt.want)
			}
		})
	}
}

func TestConvertToTheKindItAlreadyHasReturnsTheSameNode(t *testing.T) {
	t.Parallel()

	// Identity is what tells the layer above that nothing happened: a type
	// chosen from the menu that the node already has must not become an entry
	// in the history.
	nodes := []Node{
		str(t, "abc"),
		NewNumber("8080"),
		NewBool(true),
		NewNull(),
		obj(t, Member{Key: "a", Value: NewNumber("1")}),
		NewArray([]Node{NewNumber("1")}),
	}

	for _, n := range nodes {
		t.Run(n.Kind().String(), func(t *testing.T) {
			t.Parallel()

			got, err := Convert(n, n.Kind())
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}

			if got != n {
				t.Errorf("Convert returned a new %v, want the node itself", n.Kind())
			}
		})
	}
}

func TestConvertRefusesAKindThatIsNotOne(t *testing.T) {
	t.Parallel()

	// Kind is exported, so a value outside the enum can be spelled. Answering
	// with a node-less success would hand nil to whatever is about to install
	// it, and ChangeType would report an edit that produced no tree.
	defer func() {
		if recover() == nil {
			t.Error("Convert accepted a kind that is not one")
		}
	}()

	Convert(NewNull(), Kind(255)) //nolint:errcheck // it panics
}

func TestCountDescendantsCountsTheWholeSubtree(t *testing.T) {
	t.Parallel()

	empty := obj(t)
	inner := obj(t,
		Member{Key: "host", Value: str(t, "localhost")},
		Member{Key: "port", Value: NewNumber("8080")},
	)
	list := NewArray([]Node{inner, NewNull()})

	tests := []struct {
		name string
		node Node
		want int
	}{
		{name: "a scalar has none", node: NewNumber("1"), want: 0},
		{name: "an empty object has none", node: empty, want: 0},
		{name: "an empty array has none", node: NewArray(nil), want: 0},
		{name: "an object counts its members", node: inner, want: 2},
		{
			// Two elements, plus the two members of the first one.
			name: "an array counts what its elements hold",
			node: list,
			want: 4,
		},
		{
			name: "nesting is counted all the way down",
			node: obj(t,
				Member{Key: "server", Value: inner},
				Member{Key: "features", Value: list},
			),
			// The two members, plus what each of them holds.
			want: 2 + 2 + 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := CountDescendants(tt.node); got != tt.want {
				t.Errorf("CountDescendants = %d, want %d", got, tt.want)
			}
		})
	}
}
