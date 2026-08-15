package filestore

import (
	"bytes"
	"errors"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ytakahashi/pino/internal/application"
)

var (
	// errShortWrite reports a write that took fewer bytes than it was given.
	errShortWrite = errors.New("filestore: not all of the document was written")

	// errNotWrittenBack reports a file that reads back as something other
	// than what was written to it.
	errNotWrittenBack = errors.New("filestore: the file does not hold what was written")
)

// newFilePerm is what a file pino creates asks for. The umask has the final
// say, which is what makes a new file look like every other file the user
// creates rather than like one pino decided the mode of.
const newFilePerm fs.FileMode = 0o666

// tempAttempts is how many names are tried before giving up. A collision
// needs another process to have taken a random name in the same directory
// during this write, so more than one attempt is already generous.
const tempAttempts = 10

// fileOps are the steps a write is made of.
//
// They are gathered into a value so that a test can replace exactly one of
// them and see what a failure at that point leaves behind. The alternative —
// an interface over the file system — would put a seam through the whole
// package for the sake of six functions, and would let the production path
// be something other than the one being tested.
//
// The production Store never holds one of these. It calls writeAtomic with
// the real operations, so there is nothing to configure and nothing to get
// wrong when wiring pino up.
type fileOps struct {
	create  func(name string, perm fs.FileMode) (*os.File, error)
	write   func(f *os.File, data []byte) (int, error)
	chmod   func(f *os.File, perm fs.FileMode) error
	sync    func(f *os.File) error
	verify  func(name string, want []byte) error
	close   func(f *os.File) error
	rename  func(from, to string) error
	syncDir func(name string) error
	remove  func(name string) error
}

func defaultOps() fileOps {
	return fileOps{
		// O_EXCL is what makes the name this write's own: a file another
		// process created under it is a refusal rather than something to
		// truncate.
		create: func(name string, perm fs.FileMode) (*os.File, error) {
			return os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		},
		write:   (*os.File).Write,
		chmod:   (*os.File).Chmod,
		sync:    (*os.File).Sync,
		verify:  verifyContents,
		close:   (*os.File).Close,
		rename:  os.Rename,
		syncDir: syncDirectory,
		remove:  os.Remove,
	}
}

// writeAtomic puts data at path by writing it somewhere else first.
//
// The bytes go to a temporary file in the same directory, which is then
// renamed over the destination. Renaming within a directory replaces the name
// in one step, so a reader either sees the whole of the old file or the whole
// of the new one, and a failure before that step leaves the original exactly
// as it was — down to its bytes and its mode.
//
// That rename is the only moment the outcome changes meaning, which is why it
// is what WriteOutcome reports. Before it, nothing has happened to the file
// the caller had; after it, the document is saved even if what follows fails.
func writeAtomic(ops fileOps, path string, data []byte) (application.WriteOutcome, error) {
	dest, err := destinationOf(path)
	if err != nil {
		return application.WriteOutcome{}, err
	}

	name, f, err := createTemp(ops, dest)
	if err != nil {
		return application.WriteOutcome{}, err
	}

	if err := fill(ops, f, name, dest, data); err != nil {
		// The file is closed and taken away whatever went wrong, so that a
		// refused save leaves the directory as it found it. Both are already
		// best-effort: the write being reported is the failure above, and a
		// temporary file that cannot be removed is not something the person
		// saving can act on.
		_ = ops.close(f)
		_ = ops.remove(name)

		return application.WriteOutcome{}, err
	}

	if err := ops.rename(name, dest.path); err != nil {
		_ = ops.remove(name)

		return application.WriteOutcome{}, err
	}

	// Past this point the document is saved. The temporary name no longer
	// exists, so there is nothing left to clean up either.
	outcome := application.WriteOutcome{Meta: summarise(data), Committed: true}

	// The file's own bytes are on disk; this is what puts its name there too,
	// so that the rename survives a crash rather than leaving the directory
	// pointing at the file that was replaced.
	if err := ops.syncDir(filepath.Dir(dest.path)); err != nil {
		return outcome, err
	}

	return outcome, nil
}

// destination is the file a write will replace.
type destination struct {
	// path is the file itself, with any symbolic links resolved. Renaming
	// over the link would replace the link with a regular file, and the
	// document was read through it, so it is the file at the end that is
	// updated.
	path string

	// perm is what the replacement should be readable and writable by.
	perm fs.FileMode

	// keep says perm was taken from a file that is already there and has to
	// be reproduced exactly. Otherwise perm is what a new file asks for, and
	// the umask narrows it.
	keep bool
}

// destinationOf works out what replacing path means, and refuses the paths
// that cannot be replaced this way.
func destinationOf(path string) (destination, error) {
	info, err := os.Lstat(path)

	switch {
	case errors.Is(err, fs.ErrNotExist):
		// Nothing is there, so there is no mode to carry over and no link to
		// follow. Creating the parent directory is not pino's to do: a path
		// typed with a directory that does not exist is a mistake worth
		// reporting, not a tree to build.
		return destination{path: path, perm: newFilePerm}, nil

	case err != nil:
		return destination{}, err
	}

	if info.Mode()&fs.ModeSymlink != 0 {
		// A link to nothing fails here rather than being treated as a new
		// file: what the link points at is where the document would go, and
		// pino has not been asked to decide that it should not exist. It is
		// reported the way reading one is, so that a broken link reads as a
		// broken link wherever it is met.
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return destination{}, brokenLinkError("write", path, err)
		}

		path = resolved

		if info, err = os.Stat(path); err != nil {
			return destination{}, err
		}
	}

	switch {
	case info.IsDir():
		return destination{}, &fs.PathError{Op: "write", Path: path, Err: errIsDirectory}

	case !info.Mode().IsRegular():
		// A device or a socket cannot be replaced by renaming another file
		// over it without becoming a regular file, which is not what the
		// person saving asked for.
		return destination{}, &fs.PathError{Op: "write", Path: path, Err: errNotRegular}
	}

	return destination{path: path, perm: info.Mode().Perm(), keep: true}, nil
}

// createTemp opens the file the bytes are written to before they become the
// document.
func createTemp(ops fileOps, dest destination) (string, *os.File, error) {
	var err error

	for range tempAttempts {
		name := tempName(dest.path)

		var f *os.File

		if f, err = ops.create(name, dest.perm); err == nil {
			return name, f, nil
		}

		// Only a name already taken is worth another try. Anything else —
		// a directory that is not there, a directory that cannot be written
		// to — will say the same thing however many names are tried.
		if !errors.Is(err, fs.ErrExist) {
			return "", nil, err
		}
	}

	return "", nil, err
}

// tempName is a name for the temporary file, in the directory the document
// will end up in.
//
// The directory is the destination's because a rename only replaces a name in
// one step within a single file system, and a temporary directory elsewhere
// is not guaranteed to be on the same one. The name is hidden and says which
// program left it, so that one surviving a crash can be recognised for what
// it is rather than looked at as a document.
//
// Nothing rests on the number being hard to guess. What makes the name this
// write's own is the O_EXCL it is created with: a name another process
// already holds is refused rather than taken over, and another is tried.
func tempName(target string) string {
	dir, base := filepath.Split(target)

	return filepath.Join(dir, "."+base+".pino"+strconv.FormatUint(rand.Uint64(), 36))
}

// fill writes data to the open temporary file and makes sure that is what it
// holds.
func fill(ops fileOps, f *os.File, name string, dest destination, data []byte) error {
	n, err := ops.write(f, data)
	if err != nil {
		return err
	}

	// A write that took some of the bytes reports no error of its own. Left
	// alone it would be renamed into place as a truncated document.
	if n != len(data) {
		return errShortWrite
	}

	// The mode is set on the open file rather than asked for at creation,
	// because the umask narrows what creation asks for. Reproducing the
	// original's mode means reproducing it exactly, umask or no umask; a new
	// file wants the opposite and is left alone.
	if dest.keep {
		if err := ops.chmod(f, dest.perm); err != nil {
			return err
		}
	}

	// The bytes reach the disk before the rename, so that a crash cannot
	// leave the name pointing at a file whose contents never arrived.
	if err := ops.sync(f); err != nil {
		return err
	}

	// Read back what was written. The document was checked before it got
	// here — encoded, parsed again and compared — so what this adds is the
	// other half: that the file system took what pino handed it. It is the
	// last moment a disagreement can be found while the original is still
	// untouched.
	if err := ops.verify(name, data); err != nil {
		return err
	}

	// Closing can report a write that only failed on its way out of the
	// buffers, so it is checked like any other step rather than deferred.
	return ops.close(f)
}

// verifyContents reads name and reports whether it holds want.
func verifyContents(name string, want []byte) error {
	got, err := os.ReadFile(name)
	if err != nil {
		return err
	}

	if !bytes.Equal(got, want) {
		return errNotWrittenBack
	}

	return nil
}

// syncDirectory puts a directory's own contents — the names in it — on disk.
func syncDirectory(name string) error {
	d, err := os.Open(name)
	if err != nil {
		return err
	}

	if err := d.Sync(); err != nil {
		_ = d.Close()

		return err
	}

	return d.Close()
}
