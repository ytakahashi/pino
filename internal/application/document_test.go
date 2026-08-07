package application

import "testing"

func TestDocumentIsDirty(t *testing.T) {
	t.Parallel()

	root := text(t, "a")
	doc := NewDocument(root)

	if doc.Root() != root {
		t.Errorf("Root() = %v, want the tree it was opened with", doc.Root())
	}

	if doc.IsDirty() {
		t.Error("IsDirty() = true just after opening, want false")
	}

	// An edit replaces the root; here it stands in for one.
	doc.root = text(t, "b")

	if !doc.IsDirty() {
		t.Error("IsDirty() = false after the root changed, want true")
	}

	// Undoing back to the original root has to clear the mark, which is the
	// reason the comparison is on the trees rather than on an edit counter.
	doc.root = root

	if doc.IsDirty() {
		t.Error("IsDirty() = true after returning to the saved tree, want false")
	}
}
