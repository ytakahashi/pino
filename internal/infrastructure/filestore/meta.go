package filestore

import (
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

var (
	// errNoMeta reports that there is nothing recorded to compare against.
	errNoMeta = errors.New("filestore: nothing recorded about this file")

	// errForeignMeta reports a Meta this store did not issue.
	errForeignMeta = errors.New("filestore: meta was not issued by this store")
)

// fromMeta recovers what Read recorded.
//
// A nil Meta is not a claim about the file but the absence of one. The port
// allows it and gives it a meaning of its own, "nothing known about the file
// yet", which is where a document that never came from disk starts. It is
// reported separately so that what an unknown file means is decided by the
// caller and not here, where the two would be indistinguishable.
//
// A Meta the store did not issue says nothing about the file either, but for
// the opposite reason: the caller carried the wrong value, which is a mistake
// rather than a change.
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
