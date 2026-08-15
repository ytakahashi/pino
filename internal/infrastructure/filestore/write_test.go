package filestore

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/ytakahashi/pino/internal/application"
)

func TestWriteReplacesTheContents(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "config.json", []byte(`{"port":8080}`))
	data := []byte("{\n  \"port\": 8081\n}\n")

	outcome, err := New().Write(path, data)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	if !outcome.Committed {
		t.Error("Write reported nothing committed after replacing a file")
	}

	if got := contentsOf(t, path); !bytes.Equal(got, data) {
		t.Errorf("the file holds %q, want %q", got, data)
	}

	// The Meta describes the file that is now there, so that the next save
	// compares against what was written rather than against what was read.
	_, m := readFile(t, path)
	if recorded(t, outcome.Meta) != recorded(t, m) {
		t.Error("the Meta from Write describes something other than the file it wrote")
	}
}

// Empty and large documents go through the same steps as any other. The
// first is what a file becomes when everything in it is deleted; the second
// is where a write that only takes part of the bytes would show up.
func TestWriteHandlesDocumentsOfAnySize(t *testing.T) {
	tests := map[string][]byte{
		"nothing at all":  nil,
		"a single byte":   []byte("1"),
		"a long document": bytes.Repeat([]byte(`{"a":1},`), 200_000),
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "config.json", []byte("before"))

			if _, err := New().Write(path, data); err != nil {
				t.Fatalf("Write: %v", err)
			}

			if got := contentsOf(t, path); !bytes.Equal(got, data) {
				t.Errorf("the file holds %d bytes, want %d", len(got), len(data))
			}
		})
	}
}

// Replacing a file by renaming another one over it is where a mode is easily
// lost: what arrives is the temporary file's, not the original's.
func TestWriteKeepsTheModeOfTheFileItReplaces(t *testing.T) {
	for _, mode := range []fs.FileMode{0o600, 0o640, 0o644, 0o664, 0o755} {
		t.Run(mode.String(), func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "config.json", []byte("before"))

			if err := os.Chmod(path, mode); err != nil {
				t.Fatalf("Chmod: %v", err)
			}

			if _, err := New().Write(path, []byte("after")); err != nil {
				t.Fatalf("Write: %v", err)
			}

			if got := modeOf(t, path); got != mode {
				t.Errorf("the file is %v, want the %v it was", got, mode)
			}
		})
	}
}

// A file pino creates should look like every other file the user creates,
// which means the umask decides and pino does not.
func TestWriteLetsTheUmaskDecideANewFilesMode(t *testing.T) {
	for _, umask := range []int{0o022, 0o077, 0o002} {
		t.Run(fs.FileMode(umask).String(), func(t *testing.T) {
			previous := syscall.Umask(umask)
			defer syscall.Umask(previous)

			path := missing(t.TempDir())

			if _, err := New().Write(path, []byte("{}")); err != nil {
				t.Fatalf("Write: %v", err)
			}

			if got, want := modeOf(t, path), newFilePerm&^fs.FileMode(umask); got != want {
				t.Errorf("the new file is %v, want %v", got, want)
			}
		})
	}
}

// The parent is not created. A path typed with a directory that does not
// exist is a mistake worth reporting, not a tree to build.
func TestWriteDoesNotCreateTheParentDirectory(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "missing")

	outcome, err := New().Write(filepath.Join(parent, "config.json"), []byte("{}"))

	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Write returned %v, want a not-exist error", err)
	}

	if outcome.Committed {
		t.Error("Write reported a commit after failing to create the file")
	}

	if _, err := os.Stat(parent); !errors.Is(err, fs.ErrNotExist) {
		t.Error("Write created the parent directory")
	}
}

// The document was read through the link, so it is written through it too.
// Renaming over the link would replace it with a regular file, which is a
// change to the user's directory that nobody asked for.
func TestWriteUpdatesWhatASymlinkPointsAt(t *testing.T) {
	tests := map[string]func(t *testing.T, dir, target string) string{
		"an absolute link": func(t *testing.T, dir, target string) string {
			t.Helper()

			link := filepath.Join(dir, "link.json")
			symlink(t, target, link)

			return link
		},
		"a relative link": func(t *testing.T, dir, target string) string {
			t.Helper()

			link := filepath.Join(dir, "link.json")
			symlink(t, filepath.Base(target), link)

			return link
		},
		"a link into another directory": func(t *testing.T, dir, target string) string {
			t.Helper()

			sub := filepath.Join(dir, "sub")
			if err := os.Mkdir(sub, 0o755); err != nil {
				t.Fatalf("Mkdir: %v", err)
			}

			link := filepath.Join(sub, "link.json")
			symlink(t, target, link)

			return link
		},
	}

	for name, makeLink := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			target := writeFile(t, dir, "config.json", []byte("before"))
			link := makeLink(t, dir, target)
			data := []byte("after")

			if _, err := New().Write(link, data); err != nil {
				t.Fatalf("Write: %v", err)
			}

			if !isSymlink(t, link) {
				t.Error("the link itself was replaced by a regular file")
			}

			if got := contentsOf(t, target); !bytes.Equal(got, data) {
				t.Errorf("the target holds %q, want %q", got, data)
			}

			if left := leftovers(t, filepath.Dir(target), filepath.Base(target)); len(left) != 0 {
				t.Errorf("the temporary file was left in the target's directory: %v", left)
			}
		})
	}
}

// What cannot be replaced by renaming a file over it is refused before
// anything is created, rather than turned into a regular file.
func TestWriteRefusesWhatItCannotReplace(t *testing.T) {
	tests := map[string]struct {
		set  func(t *testing.T, dir string) string
		want error
	}{
		"a directory": {
			set: func(t *testing.T, dir string) string {
				t.Helper()

				path := filepath.Join(dir, "sub")
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("Mkdir: %v", err)
				}

				return path
			},
			want: errIsDirectory,
		},
		"a named pipe": {
			set: func(t *testing.T, dir string) string {
				t.Helper()

				path := filepath.Join(dir, "pipe")
				if err := syscall.Mkfifo(path, 0o644); err != nil {
					t.Fatalf("Mkfifo: %v", err)
				}

				return path
			},
			want: errNotRegular,
		},

		// The link points at nothing. Where the document would go is what the
		// link says, and pino has not been asked to decide that it should not
		// exist. It is refused in the same words reading one is refused in,
		// and not as a path that simply holds nothing — that is what a new
		// file is created from.
		"a broken symlink": {
			set: func(t *testing.T, dir string) string {
				t.Helper()

				path := filepath.Join(dir, "link.json")
				symlink(t, filepath.Join(dir, "gone"), path)

				return path
			},
			want: errBrokenSymlink,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := tc.set(t, dir)

			outcome, err := New().Write(path, []byte("{}"))

			if !errors.Is(err, tc.want) {
				t.Errorf("Write returned %v, want %v", err, tc.want)
			}

			if outcome.Committed {
				t.Error("Write reported a commit for a path it refused")
			}

			if left := leftovers(t, dir, filepath.Base(path)); len(left) != 0 {
				t.Errorf("a temporary file was left behind: %v", left)
			}
		})
	}
}

// Every step before the rename is a step at which the original must still be
// exactly what it was: its bytes, its mode, and no debris beside it.
func TestWriteLeavesTheOriginalUntouchedWhenAStepFails(t *testing.T) {
	original := []byte(`{"port":8080}`)

	for _, step := range preCommitSteps() {
		t.Run(step+" fails", func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "config.json", original)

			if err := os.Chmod(path, 0o640); err != nil {
				t.Fatalf("Chmod: %v", err)
			}

			outcome, err := writeAtomic(opsFailingAt(step), path, []byte("{\n  \"port\": 8081\n}\n"))

			if err == nil {
				t.Fatal("Write returned no error although a step failed")
			}

			if outcome.Committed {
				t.Error("Write reported a commit although the rename had not happened")
			}

			if outcome.Meta != nil {
				t.Error("Write returned a Meta although nothing was written")
			}

			if got := contentsOf(t, path); !bytes.Equal(got, original) {
				t.Errorf("the original now holds %q, want the %q it held", got, original)
			}

			if got := modeOf(t, path); got != 0o640 {
				t.Errorf("the original is now %v, want the %v it was", got, 0o640)
			}

			if left := leftovers(t, dir, "config.json"); len(left) != 0 {
				t.Errorf("a temporary file was left behind: %v", left)
			}
		})
	}
}

// A short write reports no error of its own, so it has to be noticed rather
// than reported. Nothing else would stop a truncated document being renamed
// into place.
func TestWriteRefusesToCommitPartOfADocument(t *testing.T) {
	dir := t.TempDir()
	original := []byte(`{"port":8080}`)
	path := writeFile(t, dir, "config.json", original)

	_, err := writeAtomic(opsFailingAt("a short write"), path, []byte("{\n  \"port\": 8081\n}\n"))

	if !errors.Is(err, errShortWrite) {
		t.Errorf("Write returned %v, want errShortWrite", err)
	}

	if got := contentsOf(t, path); !bytes.Equal(got, original) {
		t.Errorf("the original now holds %q, want the %q it held", got, original)
	}
}

// A file that reads back as something else is the one thing a write cannot
// find out by being told it succeeded.
func TestWriteReadsBackWhatItWrote(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "config.json", []byte("before"))

	ops := defaultOps()

	var checked []byte

	ops.verify = func(name string, want []byte) error {
		checked = want

		return verifyContents(name, want)
	}

	data := []byte("after")

	if _, err := writeAtomic(ops, path, data); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if !bytes.Equal(checked, data) {
		t.Error("the bytes read back were not the ones the document was written from")
	}

	// The check is the real one, against a file that is not what was asked
	// for.
	if err := verifyContents(path, []byte("something else")); !errors.Is(err, errNotWrittenBack) {
		t.Errorf("verifying wrong contents returned %v, want errNotWrittenBack", err)
	}
}

// The temporary file has to be in the destination's own directory: a rename
// only replaces a name in one step within a single file system, and anywhere
// else is not guaranteed to be on the same one. Until that rename, a reader
// still sees the whole of the old file.
func TestWriteStagesTheDocumentBesideTheDestination(t *testing.T) {
	dir := t.TempDir()
	original := []byte(`{"port":8080}`)
	path := writeFile(t, dir, "config.json", original)

	ops := defaultOps()

	var staged string

	ops.rename = func(from, to string) error {
		staged = from

		if got := contentsOf(t, path); !bytes.Equal(got, original) {
			t.Errorf("the file held %q before the rename, want the %q it started with", got, original)
		}

		return os.Rename(from, to)
	}

	data := []byte("{\n  \"port\": 8081\n}\n")

	if _, err := writeAtomic(ops, path, data); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if got, want := filepath.Dir(staged), filepath.Dir(path); got != want {
		t.Errorf("the document was staged in %s, want %s", got, want)
	}

	if staged == path {
		t.Error("the document was staged at the destination itself")
	}

	if got := contentsOf(t, path); !bytes.Equal(got, data) {
		t.Errorf("the file holds %q after the rename, want %q", got, data)
	}
}

// A temporary name another process has taken is tried again rather than
// truncated, and a name that cannot be created for any other reason is not
// tried ten times over.
func TestWriteFindsAnotherNameWhenOneIsTaken(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "config.json", []byte("before"))

	ops := defaultOps()
	real := ops.create
	attempts := 0

	ops.create = func(name string, perm fs.FileMode) (*os.File, error) {
		attempts++

		if attempts < 3 {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrExist}
		}

		return real(name, perm)
	}

	if _, err := writeAtomic(ops, path, []byte("after")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if attempts != 3 {
		t.Errorf("the name was tried %d times, want 3", attempts)
	}

	ops.create = func(string, fs.FileMode) (*os.File, error) { return nil, errInjected }

	if _, err := writeAtomic(ops, path, []byte("after")); !errors.Is(err, errInjected) {
		t.Errorf("Write returned %v, want the failure the first attempt reported", err)
	}
}

// After the rename the document is saved, whatever happens next. Reporting it
// as unsaved would have pino offer to write it again — and find its own bytes
// there, as a change made outside.
func TestWriteReportsADocumentSavedButNotConfirmedDurable(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "config.json", []byte("before"))
	data := []byte("after")

	outcome, err := writeAtomic(opsFailingAt("the directory sync"), path, data)

	if !errors.Is(err, errInjected) {
		t.Errorf("Write returned %v, want the failure the sync reported", err)
	}

	if !outcome.Committed {
		t.Error("Write reported nothing committed although the rename had happened")
	}

	if outcome.Meta == nil {
		t.Error("Write reported a commit without the Meta of what it wrote")
	}

	if got := contentsOf(t, path); !bytes.Equal(got, data) {
		t.Errorf("the file holds %q, want the %q that was committed", got, data)
	}

	if left := leftovers(t, dir, "config.json"); len(left) != 0 {
		t.Errorf("a temporary file was left behind: %v", left)
	}
}

// The port allows three outcomes and no others; a store producing anything
// else would leave the layer above deciding what a fourth one means.
func TestWriteNeverReportsACommitWithoutMetaOrAFailureWithOne(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "config.json", []byte("before"))

	steps := append(preCommitSteps(), "the directory sync")

	for _, step := range steps {
		t.Run(step+" fails", func(t *testing.T) {
			outcome, err := writeAtomic(opsFailingAt(step), path, []byte("after"))

			if err == nil {
				t.Fatal("Write returned no error although a step failed")
			}

			if outcome.Committed != (outcome.Meta != nil) {
				t.Errorf("Write returned Committed=%v with Meta=%v", outcome.Committed, outcome.Meta)
			}
		})
	}

	outcome, err := New().Write(path, []byte("after again"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	if !outcome.Committed || outcome.Meta == nil {
		t.Errorf("a write that succeeded returned Committed=%v Meta=%v", outcome.Committed, outcome.Meta)
	}
}

// The port is satisfied by the real store, which is what the application is
// given.
func TestStoreSatisfiesTheWritingHalfOfThePort(t *testing.T) {
	var store application.FileStore = New()

	dir := t.TempDir()
	path := writeFile(t, dir, "config.json", []byte("before"))

	outcome, err := store.Write(path, []byte("after"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	status, err := store.HasChangedSince(path, outcome.Meta)
	if err != nil {
		t.Fatalf("HasChangedSince: %v", err)
	}

	// The Meta a write hands back describes the file it left behind, so
	// saving twice in a row does not report pino's own write as a change made
	// outside.
	if status != application.ChangeNone {
		t.Errorf("HasChangedSince = %v against the Meta from Write, want %v", status, application.ChangeNone)
	}
}
