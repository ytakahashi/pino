package application

import "github.com/ytakahashi/pino/internal/domain"

// Document is the tree pino is showing, next to the tree that is on disk.
//
// Editing replaces the root with a new one that shares every untouched
// subtree, so the two fields are usually the same pointer or differ by a
// handful of nodes.
type Document struct {
	root      domain.Node
	savedRoot domain.Node
}

// NewDocument opens root as both the current and the saved tree.
//
// A document that has never been written starts here as well, with the empty
// object it was created from as its saved tree: nothing has been edited yet,
// so nothing should be reported as unsaved.
func NewDocument(root domain.Node) *Document {
	return &Document{root: root, savedRoot: root}
}

// Root is the tree as it currently stands.
func (d *Document) Root() domain.Node { return d.root }

// Replace swaps in a new tree.
//
// The saved tree is left alone, which is what makes IsDirty tell an edit from
// the document as it was read. Undo comes through here as well as editing
// does: making an earlier version current is the same operation as making a
// new one current, since a version is a whole tree either way.
func (d *Document) Replace(root domain.Node) { d.root = root }

// IsDirty reports whether there is anything to save.
//
// The comparison is between two immutable roots, so editing and then undoing
// back to the original clears the mark, which a "something was typed" flag
// would not. Node is sealed to pointer types, which is what makes comparing
// two of them with == safe.
func (d *Document) IsDirty() bool { return d.root != d.savedRoot }
