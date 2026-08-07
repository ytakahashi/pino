// Package application drives pino: it holds the open document, the view
// state and the mode, turns a user Action into a new state, and renders the
// document to lines.
//
// It depends on domain and on the ports declared in this file, never on a
// concrete adapter. The command line assembles the adapters and injects them,
// which is what keeps the parser and the file system out of the state
// transitions and makes them testable with plain table tests.
//
// Rendering lives here rather than in presentation because cursor movement,
// scrolling and keeping the selection across a view switch all operate on the
// rendered lines. Spans carry a Role instead of a colour, so no UI library
// reaches into this layer.
package application

import "github.com/ytakahashi/pino/internal/domain"

// Parser turns the bytes of a document into a tree.
//
// The port is defined by the technology it needs rather than by the use case
// it serves: a Load(path) port would force one adapter to depend on both the
// file system and the JSON parser. Keeping them apart means the sequence
// "read the bytes, then parse them" is written here, in the open flow, and
// neither adapter has to know the other exists.
type Parser interface {
	Parse(src []byte, d domain.Dialect) (domain.Node, error)
}

// FileStore is the file system as pino uses it.
//
// Encoding has no port: it is a pure function over the tree and lives in
// domain. Parsing needs an outside library and encoding does not, so making
// the two symmetric would only add an indirection that buys nothing.
type FileStore interface {
	// Read returns the contents of path together with the Meta to hand back
	// to HasChangedSince later. The Meta may be nil, which HasChangedSince
	// then has to treat as "nothing known about the file yet".
	Read(path string) ([]byte, Meta, error)

	// Write replaces the contents of path.
	Write(path string, data []byte) error

	// HasChangedSince reports whether path still holds what it held when m
	// was taken. It is consulted immediately before writing, so that pino
	// does not silently overwrite an edit made elsewhere.
	//
	// A Meta the store did not produce is an error rather than a change: it
	// means the wrong value was carried, not that the file moved.
	HasChangedSince(path string, m Meta) (ChangeStatus, error)
}

// Meta is what a FileStore remembers about a file between reading it and
// writing it back.
//
// It is opaque on purpose: modification time, size and hash are file system
// concepts, and letting them into this layer would mean the open and save
// flows start making decisions about them. The application only carries the
// value from Read back to HasChangedSince; the store that produced it is the
// one that reads it, by type assertion.
//
// The opacity is not enforced by the type. A marker interface would seal it
// tighter but cannot be satisfied from outside: an unexported method is
// qualified by the package declaring it, so a store's own fileMeta would be a
// different method and the store could not implement the port at all. What
// keeps the contents unreadable here is the rule that this layer does not
// import the store, and so cannot name the type to assert it back to.
type Meta any

// ChangeStatus is what happened to a file since its Meta was taken.
type ChangeStatus int

const (
	ChangeNone ChangeStatus = iota
	ChangeModified
	ChangeDeleted
)

func (c ChangeStatus) String() string {
	switch c {
	case ChangeNone:
		return "unchanged"
	case ChangeModified:
		return "modified"
	case ChangeDeleted:
		return "deleted"
	default:
		return "unknown"
	}
}
