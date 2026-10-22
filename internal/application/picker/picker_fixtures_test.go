package picker

import (
	"slices"
	"testing"
)

// fakeDirLister answers each call with the next of its results, repeating the
// last once they run out, and records the directories it was asked for. That
// is what lets a test change the directory between the first listing and the
// one read again after a failure.
type fakeDirLister struct {
	results []listResult
	dirs    []string
}

type listResult struct {
	entries []DirEntry
	err     error
}

func (f *fakeDirLister) List(dir string) ([]DirEntry, error) {
	f.dirs = append(f.dirs, dir)
	r := f.results[min(len(f.dirs), len(f.results))-1]

	return slices.Clone(r.entries), r.err
}

// listing is a lister whose every answer is files named names.
func listing(names ...string) *fakeDirLister {
	return &fakeDirLister{results: []listResult{{entries: files(names...)}}}
}

func files(names ...string) []DirEntry {
	entries := make([]DirEntry, len(names))
	for i, name := range names {
		entries[i] = DirEntry{Name: name}
	}

	return entries
}

// newPicker lists dir from l and fails the test if that fails.
func newPicker(t *testing.T, l DirLister, dir string) *Picker {
	t.Helper()

	p, err := New(l, dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return p
}

// selected is the name under the cursor in f, or "" when there is none.
func selected(f Frame) string {
	if f.Cursor < 0 {
		return ""
	}

	return f.Names[f.Cursor]
}

// numbered is n file names in the order the picker sorts them.
func numbered(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = string(rune('a'+i)) + ".json"
	}

	return names
}
