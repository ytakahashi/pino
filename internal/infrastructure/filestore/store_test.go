package filestore

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

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

// readFile reads path through a Store, failing the test if it will not.
func readFile(t *testing.T, path string) ([]byte, application.Meta) {
	t.Helper()

	data, m, err := New().Read(path)
	if err != nil {
		t.Fatalf("Read(%s): %v", path, err)
	}

	return data, m
}

// TestReadReturnsContentsVerbatim covers the store not knowing what is in the
// files it reads. Nothing here is JSON, and none of it may be inspected,
// rejected or repaired on the way through.
func TestReadReturnsContentsVerbatim(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{"json", []byte(`{"a":1}`)},
		{"empty", []byte{}},
		{"not json at all", []byte("hello")},
		{"invalid utf-8", []byte("\xff\xfe\x00\x01")},
		{"nul bytes", []byte("a\x00b")},
		{"crlf newlines", []byte("{\r\n  \"a\": 1\r\n}\r\n")},
		{"no trailing newline", []byte(`{"a":1}`)},
		{"long", bytes.Repeat([]byte("0123456789"), 10000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, t.TempDir(), "f", tt.content)

			data, _ := readFile(t, path)
			if !bytes.Equal(data, tt.content) {
				t.Errorf("Read returned %q, want %q", data, tt.content)
			}
		})
	}
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

// TestReadMetaDescribesTheContents checks that what is recorded is the hash of
// what was read, and only that.
func TestReadMetaDescribesTheContents(t *testing.T) {
	content := []byte(`{"a":1}`)
	path := writeFile(t, t.TempDir(), "f", content)

	_, m := readFile(t, path)

	if want := (meta{hash: sha256.Sum256(content)}); recorded(t, m) != want {
		t.Errorf("recorded %x, want %x", recorded(t, m).hash, want.hash)
	}
}

// TestReadMetaDistinguishesContents covers the reason a hash is what is kept:
// two files can agree on everything cheaper to compare and still differ.
func TestReadMetaDistinguishesContents(t *testing.T) {
	dir := t.TempDir()
	first := writeFile(t, dir, "first", []byte(`{"a":1}`))
	second := writeFile(t, dir, "second", []byte(`{"a":2}`))

	_, m1 := readFile(t, first)
	_, m2 := readFile(t, second)

	if sizeOf(t, first) != sizeOf(t, second) {
		t.Fatal("the fixtures differ in size, which is not what this test is about")
	}

	if recorded(t, m1) == recorded(t, m2) {
		t.Errorf("different contents recorded the same hash %x", recorded(t, m1).hash)
	}
}

func sizeOf(t *testing.T, path string) int64 {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}

	return info.Size()
}

// TestReadMetaIgnoresTimestamps is the property that recording only a hash
// buys. A file whose modification time moved without its contents changing has
// to record the same thing, or saving would stop to warn about an edit that
// nobody made.
func TestReadMetaIgnoresTimestamps(t *testing.T) {
	path := writeFile(t, t.TempDir(), "f", []byte(`{"a":1}`))

	_, before := readFile(t, path)

	touched := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, touched, touched); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	_, after := readFile(t, path)

	if recorded(t, before) != recorded(t, after) {
		t.Error("a file that was only touched recorded something different")
	}
}

// TestReadMetaIsOpaque is what the layering rests on: the value handed out
// carries no type the layers above can take apart. They cannot name meta, so
// the assertions they could write are the ones tried here, and all of them
// have to fail.
func TestReadMetaIsOpaque(t *testing.T) {
	path := writeFile(t, t.TempDir(), "f", []byte(`{"a":1}`))

	_, m := readFile(t, path)

	if m == nil {
		t.Fatal("Read returned no Meta")
	}

	if _, ok := m.([]byte); ok {
		t.Error("Meta asserts to []byte")
	}

	if _, ok := m.(string); ok {
		t.Error("Meta asserts to string")
	}

	if _, ok := m.(time.Time); ok {
		t.Error("Meta asserts to time.Time")
	}

	if _, ok := m.(int64); ok {
		t.Error("Meta asserts to int64")
	}

	if _, ok := m.(fs.FileInfo); ok {
		t.Error("Meta asserts to fs.FileInfo")
	}
}

// lookalike has the shape of meta without being it, standing in for a Meta
// issued by some other store.
type lookalike struct {
	hash [32]byte
}

func TestFromMetaRejectsForeignValues(t *testing.T) {
	tests := []struct {
		name string
		m    application.Meta
	}{
		{"a string", "meta"},
		{"bytes", []byte("meta")},
		{"a time", time.Now()},
		{"a struct of the same shape", lookalike{hash: sha256.Sum256([]byte(`{"a":1}`))}},
		{"a pointer to meta rather than a value", &meta{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := fromMeta(tt.m); !errors.Is(err, errForeignMeta) {
				t.Errorf("fromMeta returned %v, want errForeignMeta", err)
			}
		})
	}
}

// TestFromMetaSeparatesAnUnknownFile covers the port allowing a nil Meta and
// giving it a meaning of its own. Folding it in with a Meta that is merely
// wrong would leave the save flow unable to tell "there is nothing to compare
// against" from "the caller carried the wrong value", which are answered
// differently: the first is an ordinary state for a document that never came
// from disk, the second is a bug.
func TestFromMetaSeparatesAnUnknownFile(t *testing.T) {
	_, err := fromMeta(nil)

	if !errors.Is(err, errNoMeta) {
		t.Errorf("fromMeta(nil) returned %v, want errNoMeta", err)
	}

	if errors.Is(err, errForeignMeta) {
		t.Error("fromMeta(nil) is reported as a meta from another store")
	}
}

func TestReadFailures(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f", []byte(`{"a":1}`))

	tests := []struct {
		name string
		path string
		want error
	}{
		{"a path that does not exist", filepath.Join(dir, "missing"), fs.ErrNotExist},
		{"a directory", dir, errIsDirectory},
		{"a path under a file", filepath.Join(dir, "f", "g"), syscall.ENOTDIR},

		// A character device opens and reads without blocking, which a fifo
		// would not, so it is what this case can be written against.
		{"a device", os.DevNull, errNotRegular},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, m, err := New().Read(tt.path)

			if !errors.Is(err, tt.want) {
				t.Fatalf("Read(%s) returned %v, want %v", tt.path, err, tt.want)
			}

			if data != nil || m != nil {
				t.Errorf("a failed read returned data %q and meta %v, want neither", data, m)
			}
		})
	}
}

// TestReadFailureNamesThePath covers the errors the store builds itself
// carrying the same shape as the ones the file system returns, so that one way
// of reporting them serves for both.
func TestReadFailureNamesThePath(t *testing.T) {
	dir := t.TempDir()

	_, _, err := New().Read(dir)

	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("Read returned %T, want *fs.PathError", err)
	}

	if pathErr.Path != dir {
		t.Errorf("Path = %q, want %q", pathErr.Path, dir)
	}
}

func TestReadPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, which reads a file whatever its mode says")
	}

	path := writeFile(t, t.TempDir(), "f", []byte(`{"a":1}`))
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	if _, _, err := New().Read(path); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("Read returned %v, want a permission error", err)
	}
}

// TestReadFollowsSymlinks covers a link being read as the file it points at.
// Saving resolves links too, so that renaming over the original does not
// replace the link itself.
func TestReadFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`{"a":1}`)
	target := writeFile(t, dir, "target", content)

	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	data, m := readFile(t, link)
	if !bytes.Equal(data, content) {
		t.Errorf("Read returned %q, want %q", data, content)
	}

	_, direct := readFile(t, target)

	if recorded(t, m) != recorded(t, direct) {
		t.Error("reading through the link recorded something other than reading the target")
	}
}

// TestBrokenSymlink checks that a link to nothing is reported as a missing
// file rather than as one of the store's own refusals.
func TestBrokenSymlink(t *testing.T) {
	dir := t.TempDir()

	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "missing"), link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	if _, _, err := New().Read(link); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read returned %v, want a not-exist error", err)
	}
}

// TestWritingIsNotSupportedYet fixes what the unimplemented half of the port
// does, so that a caller reaching it is told rather than quietly succeeding.
func TestWritingIsNotSupportedYet(t *testing.T) {
	path := writeFile(t, t.TempDir(), "f", []byte(`{"a":1}`))
	store := New()

	t.Run("Write", func(t *testing.T) {
		if err := store.Write(path, []byte("x")); !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("Write returned %v, want ErrUnsupported", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}

		if string(data) != `{"a":1}` {
			t.Errorf("the file was changed to %q", data)
		}
	})

	t.Run("HasChangedSince", func(t *testing.T) {
		_, m := readFile(t, path)

		status, err := store.HasChangedSince(path, m)
		if !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("HasChangedSince returned %v, want ErrUnsupported", err)
		}

		// Not ChangeNone: a caller that dropped the error would read that as
		// permission to overwrite.
		if status == application.ChangeNone {
			t.Error("HasChangedSince reported ChangeNone alongside an error")
		}
	})
}
