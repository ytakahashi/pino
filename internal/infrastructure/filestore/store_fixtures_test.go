package filestore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ytakahashi/pino/internal/application"
)

// writeFile puts content at name inside dir and returns the path.
func writeFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}

	return path
}

// overwrite replaces what is at path, standing in for another program having
// written the file while pino held it open.
func overwrite(t *testing.T, path string, content []byte) {
	t.Helper()

	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// symlink points name at target.
func symlink(t *testing.T, target, name string) {
	t.Helper()

	if err := os.Symlink(target, name); err != nil {
		t.Fatalf("Symlink(%s -> %s): %v", name, target, err)
	}
}

// readFile reads path through a Store, failing the test if it will not.
func readFile(t *testing.T, path string) ([]byte, application.Meta) {
	t.Helper()

	data, m, err := New().Read(path)
	if err != nil {
		t.Fatalf("Read(%s): %v", path, err)
	}

	return data, m
}

// recorded recovers what the store put in m, from inside the package, which is
// the only place that can see it.
func recorded(t *testing.T, m application.Meta) meta {
	t.Helper()

	got, err := fromMeta(m)
	if err != nil {
		t.Fatalf("fromMeta: %v", err)
	}

	return got
}

func sizeOf(t *testing.T, path string) int64 {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}

	return info.Size()
}
