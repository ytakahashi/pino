// Package filestore reads and writes the files pino opens.
//
// It does not know that their contents are JSON. That is what keeps the port
// split along the technology rather than the use case: reading bytes and
// parsing them are sequenced by the layer above, and neither adapter has to
// know the other exists.
package filestore

import (
	"crypto/sha256"
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
		return nil, nil, err
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
	return data, meta{hash: sha256.Sum256(data)}, nil
}

// Write replaces the contents of path.
func (s *Store) Write(path string, data []byte) error {
	return errors.ErrUnsupported
}

// HasChangedSince reports whether path still holds what it held when m was
// taken.
func (s *Store) HasChangedSince(path string, m application.Meta) (application.ChangeStatus, error) {
	// ChangeModified rather than the zero value. A caller that dropped the
	// error would otherwise read "unchanged" and overwrite whatever is there,
	// and of the two ways to be wrong that is the one that loses work.
	return application.ChangeModified, errors.ErrUnsupported
}
