package filestore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The helpers here are for the failures a real file system will not produce
// on demand. Everything about symbolic links, modes and renaming is checked
// against real files instead; only "what if this exact step fails" is faked,
// and only by replacing the one operation being asked about.

// errInjected is the failure a replaced operation reports.
var errInjected = errors.New("injected failure")

// opsFailingAt returns the real operations with one of them replaced by a
// failure. The names are the steps of a write, in the order they happen.
func opsFailingAt(step string) fileOps {
	ops := defaultOps()

	switch step {
	case "create":
		ops.create = func(string, fs.FileMode) (*os.File, error) { return nil, errInjected }

	case "write":
		ops.write = func(*os.File, []byte) (int, error) { return 0, errInjected }

	// A write that reports no error and takes only some of the bytes. Left
	// alone, a truncated document would be renamed into place.
	case "a short write":
		ops.write = func(f *os.File, data []byte) (int, error) {
			n, err := f.Write(data[:len(data)/2])
			if err != nil {
				return n, err
			}

			return n, nil
		}

	case "chmod":
		ops.chmod = func(*os.File, fs.FileMode) error { return errInjected }

	case "sync":
		ops.sync = func(*os.File) error { return errInjected }

	case "verify":
		ops.verify = func(string, []byte) error { return errInjected }

	// The file is still closed: what is being faked is a close that reports
	// a write which only failed on its way out of the buffers.
	case "close":
		ops.close = func(f *os.File) error {
			_ = f.Close()

			return errInjected
		}

	case "rename":
		ops.rename = func(string, string) error { return errInjected }

	case "the directory sync":
		ops.syncDir = func(string) error { return errInjected }

	default:
		panic("no step named " + step)
	}

	return ops
}

// preCommitSteps are the steps that happen before the rename, which is to say
// every step at which the original file must still be untouched.
func preCommitSteps() []string {
	return []string{"create", "write", "a short write", "chmod", "sync", "verify", "close", "rename"}
}

// leftovers are the files in dir that pino put there and did not take away.
// The document itself is not one of them.
func leftovers(t *testing.T, dir, document string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}

	var left []string

	for _, e := range entries {
		if e.Name() != document && strings.Contains(e.Name(), ".pino") {
			left = append(left, e.Name())
		}
	}

	return left
}

// modeOf is the permission bits of path, which a test compares against the
// ones the file had before it was written.
func modeOf(t *testing.T, path string) fs.FileMode {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}

	return info.Mode().Perm()
}

// contentsOf reads path with the standard library, so that what a test
// asserts about a file does not come back through the code under test.
func contentsOf(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}

	return data
}

// isSymlink reports whether path is a link rather than the file it names.
func isSymlink(t *testing.T, path string) bool {
	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat(%s): %v", path, err)
	}

	return info.Mode()&fs.ModeSymlink != 0
}

// missing is a path inside dir that nothing has been put at.
func missing(dir string) string { return filepath.Join(dir, "new.json") }
