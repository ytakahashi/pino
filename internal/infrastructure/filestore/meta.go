package filestore

import (
	"crypto/sha256"
	"errors"

	"github.com/ytakahashi/pino/internal/application"
)

// meta is what the store remembers about a file between reading it and
// writing it back.
//
// It holds a hash of the contents and nothing else. A modification time and a
// size were both considered and left out: a check that concludes a file is
// unchanged has to be right every time, and neither of them is. Timestamps
// are restored by the tools that copy files, and their resolution can be
// coarser than the gap between two writes; a size survives plenty of edits.
// Either would let pino overwrite someone else's work without a word, and
// what it would buy is one read of a file that is about to be written anyway.
//
// The type is unexported and handed out as an application.Meta, which is an
// empty interface. That is what makes the contents unreadable above: no other
// package can name this type, so no other package can assert a Meta back to
// it. A hash is a file system concept, and letting it into the layers above
// would mean the open and save flows start making decisions about it; those
// layers carry the value and nothing more.
//
// It is a value type, so what a caller holds is a copy it cannot alter.
type meta struct {
	hash [32]byte
}

// summarise is what the store remembers about the bytes of a file.
//
// Reading a file, comparing one and writing one all describe a file this way,
// so they say it once here. A second spelling of the same rule is a way for
// the file just written to look unlike the file just read.
func summarise(data []byte) meta {
	return meta{hash: sha256.Sum256(data)}
}

var (
	// errNoMeta reports that the caller expects no file to be there.
	errNoMeta = errors.New("filestore: no file was recorded at this path")

	// errForeignMeta reports a Meta this store did not issue.
	errForeignMeta = errors.New("filestore: meta was not issued by this store")
)

// fromMeta recovers what Read recorded.
//
// A nil Meta is a claim about the file rather than the absence of one: there
// was nothing at that path when it was read, which is where a document opened
// at a path that does not exist starts. It is reported separately because it
// is compared against the file system differently — there is no hash to check,
// only the question of whether the path is still free.
//
// A Meta the store did not issue says nothing about the file at all: the
// caller carried the wrong value, which is a mistake rather than a change.
func fromMeta(m application.Meta) (meta, error) {
	if m == nil {
		return meta{}, errNoMeta
	}

	recorded, ok := m.(meta)
	if !ok {
		return meta{}, errForeignMeta
	}

	return recorded, nil
}
