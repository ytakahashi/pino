package filestore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"github.com/ytakahashi/pino/internal/application/picker"
)

func TestListReportsFilesAndDirectoriesWithoutFilteringNames(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.json", nil)
	writeFile(t, dir, ".hidden.JSON", nil)
	writeFile(t, dir, "notes.txt", nil)
	subdir := filepath.Join(dir, "sub.json")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, subdir, "nested.json", nil)

	got := listedByName(t, dir)

	want := []picker.DirEntry{
		{Name: ".hidden.JSON"},
		{Name: "config.json"},
		{Name: "notes.txt"},
		{Name: "sub.json", Dir: true},
	}
	if !slices.Equal(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
}

func TestListClassifiesSymlinksByTheirTargetsAndOmitsSpecialFiles(t *testing.T) {
	dir := t.TempDir()
	targets := t.TempDir()
	file := writeFile(t, targets, "target", nil)
	symlink(t, file, filepath.Join(dir, "file.json"))
	symlink(t, targets, filepath.Join(dir, "directory.json"))
	symlink(t, filepath.Join(targets, "missing"), filepath.Join(dir, "broken.json"))
	symlink(t, "loop.json", filepath.Join(dir, "loop.json"))
	// A relative target is resolved from the link's directory, not the cwd.
	symlink(t, "file.json", filepath.Join(dir, "chain.json"))
	fifo := filepath.Join(dir, "pipe.json")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}
	symlink(t, "pipe.json", filepath.Join(dir, "pipe-link.json"))

	got := listedByName(t, dir)

	want := []picker.DirEntry{
		{Name: "chain.json"},
		{Name: "directory.json", Dir: true},
		{Name: "file.json"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
}

func TestListReportsAnEmptyDirectory(t *testing.T) {
	got, err := NewLister().List(t.TempDir())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List = %v, want no entries", got)
	}
}

func TestListPreservesDirectoryErrors(t *testing.T) {
	dir := t.TempDir()
	tests := map[string]struct {
		path string
		want error
	}{
		"missing directory": {filepath.Join(dir, "missing"), fs.ErrNotExist},
		"regular file":      {writeFile(t, dir, "file", nil), syscall.ENOTDIR},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := NewLister().List(tt.path)
			var pathErr *fs.PathError
			if !errors.As(err, &pathErr) {
				t.Fatalf("List error = %v, want *fs.PathError", err)
			}
			if pathErr.Path != tt.path {
				t.Errorf("error path = %q, want %q", pathErr.Path, tt.path)
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("List error = %v, want %v", err, tt.want)
			}
			if len(got) != 0 {
				t.Errorf("List = %v alongside an error, want no entries", got)
			}
		})
	}
}
