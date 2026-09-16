package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// openingStdin starts a stdin document whose writes commit successfully.
func openingStdin(t *testing.T, root domain.Node, out string) (*App, *fakeFileStore) {
	t.Helper()

	files := &fakeFileStore{
		data:    map[string][]byte{},
		outcome: WriteOutcome{Meta: writtenMeta, Committed: true},
	}
	parser := &fakeParser{root: root}
	app := New(Deps{
		Parser:   parser,
		Files:    files,
		Wrapper:  fakeTextWrap{},
		JSONView: documentview.NewJSONRenderer(),
		TreeView: documentview.NewTreeRenderer(),
	}, Config{})

	if err := app.OpenStdin([]byte(testSource), out); err != nil {
		t.Fatalf("OpenStdin: %v", err)
	}

	parser.parse = func([]byte, domain.Dialect) (domain.Node, error) { return app.doc.Root(), nil }

	return app, files
}
