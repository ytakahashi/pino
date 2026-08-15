package filestore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ytakahashi/pino/internal/application"
)

// The tests here are about the question asked immediately before a file is
// overwritten. The answer that must never be wrong is ChangeNone: everything
// else stops a save and asks the user, while that one lets it through.

func TestHasChangedSinceComparesContentsAndNothingElse(t *testing.T) {
	content := []byte(`{"port":8080}`)

	tests := map[string]struct {
		after func(t *testing.T, path string)
		want  application.ChangeStatus
	}{
		"nothing happened": {
			after: func(*testing.T, string) {},
			want:  application.ChangeNone,
		},

		// Timestamps are restored by the tools that copy files and can be
		// coarser than the gap between two writes, so a file that only looks
		// touched is not one that changed.
		"only the timestamps moved": {
			after: func(t *testing.T, path string) {
				t.Helper()

				later := time.Now().Add(2 * time.Hour)
				if err := os.Chtimes(path, later, later); err != nil {
					t.Fatalf("Chtimes: %v", err)
				}
			},
			want: application.ChangeNone,
		},

		// The same number of bytes, which is what a size would miss.
		"the contents changed without the size": {
			after: func(t *testing.T, path string) {
				t.Helper()

				overwrite(t, path, []byte(`{"port":8081}`))
			},
			want: application.ChangeModified,
		},
		"the contents grew": {
			after: func(t *testing.T, path string) {
				t.Helper()

				overwrite(t, path, []byte(`{"port":8080,"debug":true}`))
			},
			want: application.ChangeModified,
		},
		"the file was emptied": {
			after: func(t *testing.T, path string) {
				t.Helper()

				overwrite(t, path, nil)
			},
			want: application.ChangeModified,
		},
		"the file was deleted": {
			after: func(t *testing.T, path string) {
				t.Helper()

				if err := os.Remove(path); err != nil {
					t.Fatalf("Remove: %v", err)
				}
			},
			want: application.ChangeDeleted,
		},

		// Rewritten with the very bytes it held. There is nothing to warn
		// about: what would be written is what is there.
		"the file was rewritten with the same contents": {
			after: func(t *testing.T, path string) {
				t.Helper()

				overwrite(t, path, content)
			},
			want: application.ChangeNone,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeFile(t, t.TempDir(), "config.json", content)
			_, m := readFile(t, path)

			tc.after(t, path)

			status, err := New().HasChangedSince(path, m)
			if err != nil {
				t.Fatalf("HasChangedSince: %v", err)
			}

			if status != tc.want {
				t.Errorf("HasChangedSince = %v, want %v", status, tc.want)
			}
		})
	}
}

// A document opened at a path that does not exist carries no Meta, and that
// is a claim about the file like any other: the path was free. Reporting it
// as "nothing to compare" and skipping the check is how a first save
// overwrites a file another program created in the meantime.
func TestHasChangedSinceTreatsNoMetaAsAnEmptyPath(t *testing.T) {
	tests := map[string]struct {
		create func(t *testing.T, dir, path string)
		want   application.ChangeStatus
	}{
		"the path is still free": {
			create: func(*testing.T, string, string) {},
			want:   application.ChangeNone,
		},
		"a file appeared": {
			create: func(t *testing.T, _, path string) {
				t.Helper()

				overwrite(t, path, []byte(`{"a":1}`))
			},
			want: application.ChangeModified,
		},
		"an empty file appeared": {
			create: func(t *testing.T, _, path string) {
				t.Helper()

				overwrite(t, path, nil)
			},
			want: application.ChangeModified,
		},
		"a directory appeared": {
			create: func(t *testing.T, _, path string) {
				t.Helper()

				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("Mkdir: %v", err)
				}
			},
			want: application.ChangeModified,
		},
		"a symlink appeared": {
			create: func(t *testing.T, dir, path string) {
				t.Helper()

				symlink(t, writeFile(t, dir, "target", []byte(`{"a":1}`)), path)
			},
			want: application.ChangeModified,
		},

		// The link points at nothing, so opening it reports a missing file.
		// The path is taken all the same, and writing through it would follow
		// the link somewhere else entirely.
		"a broken symlink appeared": {
			create: func(t *testing.T, dir, path string) {
				t.Helper()

				symlink(t, filepath.Join(dir, "missing"), path)
			},
			want: application.ChangeModified,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "new.json")

			tc.create(t, dir, path)

			status, err := New().HasChangedSince(path, nil)
			if err != nil {
				t.Fatalf("HasChangedSince: %v", err)
			}

			if status != tc.want {
				t.Errorf("HasChangedSince = %v, want %v", status, tc.want)
			}
		})
	}
}

// A link is followed, as reading it was. What matters is the file the
// document came from, not the name it was reached by.
func TestHasChangedSinceFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := writeFile(t, dir, "target", []byte(`{"a":1}`))
	link := filepath.Join(dir, "link")

	symlink(t, target, link)

	_, m := readFile(t, link)

	status, err := New().HasChangedSince(link, m)
	if err != nil {
		t.Fatalf("HasChangedSince: %v", err)
	}

	if status != application.ChangeNone {
		t.Errorf("HasChangedSince = %v on an untouched target, want %v", status, application.ChangeNone)
	}

	overwrite(t, target, []byte(`{"a":2}`))

	if status, err = New().HasChangedSince(link, m); err != nil {
		t.Fatalf("HasChangedSince: %v", err)
	}

	if status != application.ChangeModified {
		t.Errorf("HasChangedSince = %v after the target changed, want %v", status, application.ChangeModified)
	}

	// The file the document was read from is gone, whatever the link still
	// says.
	if err := os.Remove(target); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if status, err = New().HasChangedSince(link, m); err != nil {
		t.Fatalf("HasChangedSince: %v", err)
	}

	if status != application.ChangeDeleted {
		t.Errorf("HasChangedSince = %v after the target went, want %v", status, application.ChangeDeleted)
	}
}

// What cannot be read cannot be compared. None of these is a change, so none
// of them may be reported as one — and none may be reported as ChangeNone
// either, which is the answer that would let a save proceed.
func TestHasChangedSinceRefusesWhatItCannotCompare(t *testing.T) {
	content := []byte(`{"a":1}`)

	tests := map[string]struct {
		meta func(t *testing.T, path string) application.Meta
		set  func(t *testing.T, dir string) string
		want error
	}{
		"a directory": {
			meta: func(t *testing.T, path string) application.Meta {
				t.Helper()

				_, m := readFile(t, writeFile(t, filepath.Dir(path), "other", content))

				return m
			},
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
		"a meta from somewhere else": {
			meta: func(*testing.T, string) application.Meta { return lookalike{} },
			set: func(t *testing.T, dir string) string {
				t.Helper()

				return writeFile(t, dir, "config.json", content)
			},
			want: errForeignMeta,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := tc.set(t, dir)

			status, err := New().HasChangedSince(path, tc.meta(t, path))

			if !errors.Is(err, tc.want) {
				t.Errorf("HasChangedSince returned %v, want %v", err, tc.want)
			}

			if status == application.ChangeNone {
				t.Error("HasChangedSince reported ChangeNone alongside an error")
			}
		})
	}
}

// An unreadable file is not an unchanged one. The check runs as the user
// presses the save key, long after the file was opened, so the permissions
// may have changed underneath it.
func TestHasChangedSinceReportsPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a file whatever its mode says")
	}

	dir := t.TempDir()
	path := writeFile(t, dir, "config.json", []byte(`{"a":1}`))
	_, m := readFile(t, path)

	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	status, err := New().HasChangedSince(path, m)

	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("HasChangedSince returned %v, want a permission error", err)
	}

	if status == application.ChangeNone {
		t.Error("HasChangedSince reported ChangeNone alongside an error")
	}
}
