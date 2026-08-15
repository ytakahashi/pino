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
)

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

func TestReadRefusesWhatItCannotRead(t *testing.T) {
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

func TestReadReportsPermissionDenied(t *testing.T) {
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

// TestReadReportsABrokenSymlink covers a link to nothing being told apart
// from a path that holds nothing.
//
// Opening reports both as "no such file", and a missing file is what starts a
// new document: an empty one, with nothing written until the user saves. A
// link is not that. The path is taken, and a document started there would be
// written through the link to wherever it leads, so it has to arrive as a
// refusal instead.
func TestReadReportsABrokenSymlink(t *testing.T) {
	dir := t.TempDir()

	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "missing"), link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	_, _, err := New().Read(link)

	if !errors.Is(err, errBrokenSymlink) {
		t.Errorf("Read returned %v, want a broken-link error", err)
	}

	if errors.Is(err, fs.ErrNotExist) {
		t.Error("a link to nothing is reported as a path that holds nothing")
	}

	// The path it names is the link, which is what the user typed, rather
	// than the target they may never have seen.
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) && pathErr.Path != link {
		t.Errorf("Path = %q, want %q", pathErr.Path, link)
	}
}

// A path with nothing at it stays a plain missing file, which is what a new
// document is opened from.
func TestReadReportsAMissingPathAsMissing(t *testing.T) {
	dir := t.TempDir()

	_, _, err := New().Read(filepath.Join(dir, "missing.json"))

	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read returned %v, want a not-exist error", err)
	}

	if errors.Is(err, errBrokenSymlink) {
		t.Error("a missing path is reported as a broken link")
	}
}
