package filestore

import (
	"slices"
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/application/picker"
)

// listedByName lists dir through a Lister and orders the result by name.
//
// DirLister promises no order, since the picker sorts for itself, so a test
// comparing whole lists must not depend on the one os.ReadDir happens to give.
func listedByName(t *testing.T, dir string) []picker.DirEntry {
	t.Helper()

	got, err := NewLister().List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	slices.SortFunc(got, func(a, b picker.DirEntry) int {
		return strings.Compare(a.Name, b.Name)
	})

	return got
}
