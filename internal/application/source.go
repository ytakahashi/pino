package application

import "path/filepath"

// Source is where the open document came from.
//
// It is sealed by an unexported method: the ways a document can be opened are
// decided here, and each one carries its own way of being saved back.
type Source interface {
	// Name is the short label for the document, as shown in the status bar.
	Name() string

	isSource()
}

// FileSource is a document opened from a path.
type FileSource struct {
	Path string

	// New reports that the path did not exist when pino started, so the
	// first save creates the file rather than replacing it.
	New bool
}

func (FileSource) isSource() {}

// Name is the base name of the path: the status bar has one line to share
// between the mode, the view and the document, and the directory is not what
// tells two open files apart in practice.
func (s FileSource) Name() string { return filepath.Base(s.Path) }
