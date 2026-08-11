package domain_test

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// describe is a node written the way a test can compare it, which for the
// values Convert produces is the value itself. Containers only ever come back
// empty, so their contents need no spelling out.
func describe(t *testing.T, n domain.Node) string {
	t.Helper()

	switch n.Kind() {
	case domain.KindObject:
		if n.(*domain.Object).Len() == 0 {
			return "{}"
		}

		return "{...}"

	case domain.KindArray:
		if n.(*domain.Array).Len() == 0 {
			return "[]"
		}

		return "[...]"

	case domain.KindString:
		return domain.QuoteString(n.(*domain.String).Value())

	case domain.KindNumber:
		return n.(*domain.Number).Raw()

	case domain.KindBool:
		if n.(*domain.Bool).Value() {
			return "true"
		}

		return "false"

	case domain.KindNull:
		return "null"

	default:
		t.Fatalf("describe: unknown kind %v", n.Kind())

		return ""
	}
}

func TestConvertCoversEveryPairOfKinds(t *testing.T) {
	t.Parallel()

	sources := []struct {
		name string
		node domain.Node
	}{
		{name: "string", node: str(t, "abc")},
		{name: "number", node: domain.NewNumber("8080")},
		{name: "boolean", node: domain.NewBool(true)},
		{name: "null", node: domain.NewNull()},
		{
			name: "object",
			node: obj(t, domain.Member{Key: "a", Value: domain.NewNumber("1")}),
		},
		{name: "array", node: domain.NewArray([]domain.Node{domain.NewNumber("1")})},
	}

	targets := []struct {
		name string
		kind domain.Kind
	}{
		{name: "string", kind: domain.KindString},
		{name: "number", kind: domain.KindNumber},
		{name: "boolean", kind: domain.KindBool},
		{name: "null", kind: domain.KindNull},
		{name: "object", kind: domain.KindObject},
		{name: "array", kind: domain.KindArray},
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

				got, err := domain.Convert(src.node, dst.kind)
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
		from domain.Node
		kind domain.Kind
		want string
	}{
		{
			name: "a string spelling a number",
			from: str(t, "8080"),
			kind: domain.KindNumber,
			want: "8080",
		},
		{
			name: "a string spelling no number",
			from: str(t, "abc"),
			kind: domain.KindNumber,
			want: "0",
		},
		{
			// The literal survives, so a value entered as 1.50 is still 1.50
			// after a trip through string.
			name: "a number with trailing zeros",
			from: domain.NewNumber("1.50"),
			kind: domain.KindString,
			want: `"1.50"`,
		},
		{
			name: "a number in exponent notation",
			from: domain.NewNumber("1E+10"),
			kind: domain.KindString,
			want: `"1E+10"`,
		},
		{
			name: "true seen as text",
			from: domain.NewBool(true),
			kind: domain.KindString,
			want: `"true"`,
		},
		{
			name: "false seen as text",
			from: domain.NewBool(false),
			kind: domain.KindString,
			want: `"false"`,
		},
		{
			name: "the text true",
			from: str(t, "true"),
			kind: domain.KindBool,
			want: "true",
		},
		{
			name: "the text false",
			from: str(t, "false"),
			kind: domain.KindBool,
			want: "false",
		},
		{
			name: "text that is neither",
			from: str(t, "yes"),
			kind: domain.KindBool,
			want: "false",
		},
		{
			// JSON draws no correspondence between the two, so the number does
			// not come back as 1.
			name: "a boolean seen as a number",
			from: domain.NewBool(true),
			kind: domain.KindNumber,
			want: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.Convert(tt.from, tt.kind)
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
	nodes := []domain.Node{
		str(t, "abc"),
		domain.NewNumber("8080"),
		domain.NewBool(true),
		domain.NewNull(),
		obj(t, domain.Member{Key: "a", Value: domain.NewNumber("1")}),
		domain.NewArray([]domain.Node{domain.NewNumber("1")}),
	}

	for _, n := range nodes {
		t.Run(n.Kind().String(), func(t *testing.T) {
			t.Parallel()

			got, err := domain.Convert(n, n.Kind())
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

	domain.Convert(domain.NewNull(), domain.Kind(255)) //nolint:errcheck // it panics
}

func TestCountDescendantsCountsTheWholeSubtree(t *testing.T) {
	t.Parallel()

	empty := obj(t)
	inner := obj(t,
		domain.Member{Key: "host", Value: str(t, "localhost")},
		domain.Member{Key: "port", Value: domain.NewNumber("8080")},
	)
	list := domain.NewArray([]domain.Node{inner, domain.NewNull()})

	tests := []struct {
		name string
		node domain.Node
		want int
	}{
		{name: "a scalar has none", node: domain.NewNumber("1"), want: 0},
		{name: "an empty object has none", node: empty, want: 0},
		{name: "an empty array has none", node: domain.NewArray(nil), want: 0},
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
				domain.Member{Key: "server", Value: inner},
				domain.Member{Key: "features", Value: list},
			),
			// The two members, plus what each of them holds.
			want: 2 + 2 + 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := domain.CountDescendants(tt.node); got != tt.want {
				t.Errorf("CountDescendants = %d, want %d", got, tt.want)
			}
		})
	}
}
