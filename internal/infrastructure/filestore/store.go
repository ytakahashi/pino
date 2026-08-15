// Package filestore reads and writes the files pino opens.
//
// It does not know that their contents are JSON. That is what keeps the port
// split along the technology rather than the use case: reading bytes and
// parsing them are sequenced by the layer above, and neither adapter has to
// know the other exists.
package filestore

import (
	"errors"
	"io"
	"io/fs"
	"os"

	"github.com/ytakahashi/pino/internal/application"
)

// Store is the file system as pino uses it.
type Store struct{}

func New() *Store { return &Store{} }

var _ application.FileStore = (*Store)(nil)

var (
	errIsDirectory = errors.New("is a directory")
	errNotRegular  = errors.New("not a regular file")

	// errBrokenSymlink reports a link that points at nothing. It is
	// deliberately not an fs.ErrNotExist; see brokenLinkError.
	errBrokenSymlink = errors.New("broken symbolic link")
)

// Read returns the contents of path together with the Meta to hand back
// before writing.
//
// The bytes are returned as they stand. This store does not know what is in
// them, so an empty file and one holding text in no encoding at all are read
// without complaint; it is the parser that refuses them, with a position.
func (s *Store) Read(path string) ([]byte, application.Meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, brokenLinkError("read", path, err)
	}

	defer func() { _ = f.Close() }()

	// The open file is what is asked about, rather than the path: the answer
	// then describes the file whose contents are about to be read, and not
	// another one that came to have the same name in between.
	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}

	switch {
	case info.IsDir():
		return nil, nil, &fs.PathError{Op: "read", Path: path, Err: errIsDirectory}

	case !info.Mode().IsRegular():
		// A stream is refused rather than read. Saving one is not possible
		// the way pino saves, by renaming a temporary file over the original,
		// so opening it would lead nowhere; and reading one can block with no
		// way out, the terminal interface not having started yet.
		return nil, nil, &fs.PathError{Op: "read", Path: path, Err: errNotRegular}
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, err
	}

	// The hash is taken from the bytes that were actually read, so it
	// describes what pino now holds whatever became of the file meanwhile.
	// A read torn by someone else's write leaves a hash of the mixture, which
	// will not match the file when saving comes to compare the two, so it is
	// reported rather than overwritten. Nothing about the read has to be
	// retried or verified for that to hold.
	return data, summarise(data), nil
}

// brokenLinkError tells a path that holds nothing from a link that points at
// nothing.
//
// Opening reports both as "no such file", and a missing file is how a
// document opened at a path that does not exist begins: with an empty
// document and nothing on disk. A link is not that. The path is taken, and a
// document started there would be written through the link to wherever it
// leads, so it is refused instead — under an error of its own, which no
// caller can mistake for an empty path.
//
// The original error is not wrapped, on purpose. Wrapping it would leave the
// result matching fs.ErrNotExist, which is the very thing being told apart.
func brokenLinkError(op, path string, err error) error {
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	// The link itself is asked about. Anything that is not a link, and a path
	// that has gone in the meantime, are what they were reported as.
	info, statErr := os.Lstat(path)
	if statErr != nil || info.Mode()&fs.ModeSymlink == 0 {
		return err
	}

	return &fs.PathError{Op: op, Path: path, Err: errBrokenSymlink}
}

// Write replaces the contents of path, and says whether the replacement took
// effect.
//
// How it is done is in write.go: the bytes go to a temporary file beside the
// destination and are renamed over it, so that a failure before that rename
// leaves the original exactly as it was.
func (s *Store) Write(path string, data []byte) (application.WriteOutcome, error) {
	return writeAtomic(defaultOps(), path, data)
}

// HasChangedSince reports whether path still holds what expected was taken
// from.
//
// The whole file is read and hashed. Nothing cheaper is trusted: a timestamp
// is restored by the tools that copy files and can be coarser than the gap
// between two writes, and a size survives plenty of edits. Touching a file
// without changing it therefore says nothing here, and changing it without
// changing its size still does.
//
// Every answer other than ChangeNone comes with the file left alone. This is
// asked immediately before a write, so the one answer that must never be
// wrong is "nothing has changed": an error is reported as ChangeModified, so
// that a caller which dropped it would stop rather than overwrite.
func (s *Store) HasChangedSince(path string, expected application.Meta) (application.ChangeStatus, error) {
	recorded, err := fromMeta(expected)

	switch {
	case errors.Is(err, errNoMeta):
		return statusOfUnclaimed(path)

	case err != nil:
		return application.ChangeModified, err
	}

	// Read rather than a hash taken here, so that what a file is summarised
	// as is decided in one place, and so that a directory or a stream at the
	// path is refused in the same words as when it is opened.
	data, _, err := s.Read(path)

	switch {
	// A link whose target has gone is reported here as a deletion, although
	// Read refuses it under an error of its own. The two callers are asking
	// different questions: opening decides whether to start a document at a
	// path, while this asks what became of the file a document already came
	// from — and that file is gone.
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, errBrokenSymlink):
		return application.ChangeDeleted, nil

	case err != nil:
		return application.ChangeModified, err
	}

	if summarise(data) != recorded {
		return application.ChangeModified, nil
	}

	return application.ChangeNone, nil
}

// statusOfUnclaimed answers for a document that found no file at path when it
// was opened.
//
// There is no hash to compare, so the question is whether the path is still
// free. Anything at all being there is a change: a document about to be
// created has no claim on a name another program has since taken.
//
// The link itself is asked about rather than what it points at. A dangling
// symbolic link is something at the path — writing through it would follow it
// somewhere — so it counts as the path having been taken.
func statusOfUnclaimed(path string) (application.ChangeStatus, error) {
	switch _, err := os.Lstat(path); {
	case errors.Is(err, fs.ErrNotExist):
		return application.ChangeNone, nil

	case err != nil:
		return application.ChangeModified, err
	}

	return application.ChangeModified, nil
}
