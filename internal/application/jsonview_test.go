package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

func TestJSONRender(t *testing.T) {
	t.Parallel()

	for name, doc := range documents(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			checkGolden(t, name, dumpLines(NewJSONRenderer().Render(doc.root, doc.opt)))
		})
	}
}

func TestJSONRenderWithoutDocument(t *testing.T) {
	t.Parallel()

	if lines := NewJSONRenderer().Render(nil, RenderOptions{}); lines != nil {
		t.Errorf("Render(nil) = %v, want nil", lines)
	}
}

// TestJSONRenderSharesNoSpans guards the copying in spansOf: rows are built
// from a shared label, and appending a comma in place would write into the
// row that was built before it.
func TestJSONRenderSharesNoSpans(t *testing.T) {
	t.Parallel()

	root := object(t,
		member("a", domain.NewNumber("1")),
		member("b", domain.NewNumber("2")),
	)

	lines := NewJSONRenderer().Render(root, RenderOptions{})

	if got, want := lines[1].Text(), `"a": 1,`; got != want {
		t.Errorf("first member = %q, want %q", got, want)
	}

	if got, want := lines[2].Text(), `"b": 2`; got != want {
		t.Errorf("last member = %q, want %q", got, want)
	}
}
