package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// version is a revision named after its label, with a tree of its own so that
// a test can tell which one came back.
func version(t *testing.T, label string) Revision {
	t.Helper()

	value, err := domain.NewString(label)
	if err != nil {
		t.Fatalf("NewString(%q): %v", label, err)
	}

	return Revision{Root: value, Cursor: domain.Path{}, Label: label}
}
