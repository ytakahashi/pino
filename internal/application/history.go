package application

import "github.com/ytakahashi/pino/internal/domain"

// Revision is a version of the document, together with where the reader should
// be standing when it is current.
//
// The tree is immutable, so a version is the whole document rather than a
// change to it, and undo is choosing an earlier one. There is no inverse of
// any edit anywhere in pino: an edit that could not be undone correctly would
// be an edit whose inverse was written wrongly, and none is written.
type Revision struct {
	Root domain.Node

	// Cursor is where the edit that produced this version happened, which is
	// what a reader wants to be looking at while it is taken away and put
	// back. The version a document was opened at was produced by no edit, and
	// carries the root.
	//
	// It is not "where the reader was standing", which is a thing about the
	// session rather than about the document, and which cannot be recorded
	// without writing to a version as it is left behind. Undo would then have
	// to go back to wherever the reader happened to be when they last made a
	// change there — the root, right after opening a file — and a change to a
	// part of the document nobody can see is a change nobody can check.
	//
	// It resolves in the Root of this same Revision, because that is what the
	// edit that produced it guarantees about the place it left the cursor. It
	// does not necessarily resolve in the version before this one: undoing an
	// insertion takes away the node the insertion selected.
	Cursor domain.Path

	// Label names the edit that produced this version, as "edit /server/port".
	//
	// Nothing draws it yet. It is filled in all the same because the moment a
	// version is made is the only moment anything knows what made it; a list
	// of versions added later could not work the labels out from the trees.
	Label string
}

// History is the versions of the open document, and which of them is current.
//
// The invariant that makes undo and redo simple is on Revision.Cursor: it
// resolves in the Root of the same Revision. Because the tree is immutable, a
// path that resolved when a version was made resolves for as long as that
// version exists, so restoring a position cannot fail. What may still happen
// is that the position is inside something folded, which is a matter for
// whoever puts the cursor on screen rather than for this.
//
// The zero value is a history with nothing in it: undo and redo both report
// that there is nowhere to go. That is what a session with no document open
// needs, and it is why nothing here checks whether one is open.
//
// There is no limit on how many versions are kept. Editing copies only the
// nodes along the path it changed, so a version costs the depth of the
// document rather than its size, and a cap would make how far back the reader
// can go depend on what they did earlier — including whether undoing far
// enough clears the unsaved mark, which rests on reaching the tree that was
// read from disk.
type History struct {
	entries []Revision
	cursor  int
}

// NewHistory starts a history at r, which is the document as it was opened.
//
// A history is built per document rather than carried across, for the reason
// the view state is: undoing in one file must not reach into another.
func NewHistory(r Revision) History {
	return History{entries: []Revision{r}}
}

// Push makes r the current version.
//
// Everything after the current version is a future that was not taken, and
// pushing is what decides it will not be: redoing past this point is no longer
// possible. The bound covers a history with nothing in it, where there is no
// current version to keep.
func (h *History) Push(r Revision) {
	h.entries = append(h.entries[:min(h.cursor+1, len(h.entries))], r)
	h.cursor = len(h.entries) - 1
}

// Undo steps back to the previous version, reporting false at the first one.
//
// It returns the version to restore and, separately, where the change being
// undone happened. The two come from different versions: a change belongs to
// the version it produced, and undoing it is leaving that version behind. The
// path may name nothing in the version being restored, which is a question for
// whoever puts the cursor somewhere.
func (h *History) Undo() (Revision, domain.Path, bool) {
	if h.cursor <= 0 {
		return Revision{}, domain.Path{}, false
	}

	undone := h.entries[h.cursor]
	h.cursor--

	return h.entries[h.cursor], undone.Cursor, true
}

// Redo steps forward to the version undone last, reporting false at the most
// recent one.
//
// The change being redone is the one that produced the version being restored,
// so both come from it here. That is the same rule undo follows — the place is
// always taken from the later of the two versions — and it is what makes a
// step back and a step forward land in the same spot.
func (h *History) Redo() (Revision, domain.Path, bool) {
	if h.cursor+1 >= len(h.entries) {
		return Revision{}, domain.Path{}, false
	}

	h.cursor++
	rev := h.entries[h.cursor]

	return rev, rev.Cursor, true
}
