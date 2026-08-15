package application

import "github.com/ytakahashi/pino/internal/domain"

// StatusInfo is what the status bar shows.
//
// It deliberately excludes the number of rendered lines: the presentation
// layer already holds them to draw, and counting them here would render the
// whole document a second time on every redraw.
type StatusInfo struct {
	Mode     Mode
	ViewMode ViewMode

	// Name is the label of the open document, empty when none is open.
	Name string

	// Indent is one level of indentation of the open document. The status
	// bar reports it, and the JSON view draws with it, so that what is shown
	// and what will be written stay the same thing.
	Indent string

	// Dirty reports unsaved changes.
	Dirty bool

	// New reports a document whose file has still to be created. It is
	// independent of Dirty: a new document that has not been touched has
	// nothing to save, and an edited one has both to report until the first
	// save, which clears them together.
	New bool

	// Error is why the last thing the session was asked to do failed, while
	// the message is still on screen. The bar carries it as well as the
	// dialog, since a dialog is answered and gone while the bar stays.
	Error string

	// Pointer locates the selected node, as RFC 6901 spells it: the root is
	// the empty string. How to show that is left to whoever draws the bar,
	// where "/" reads better than a blank.
	Pointer string

	// Type names the kind of the selected value: object, array, string,
	// number, boolean or null. It is empty when nothing is selected, which is
	// what tells the two apart from a root holding an object.
	Type string
}

// Status describes the session for the status bar.
func (a *App) Status() StatusInfo {
	info := StatusInfo{
		Mode:     a.Mode(),
		ViewMode: a.view.ViewMode,
		Indent:   a.format.Indent,
	}

	if a.source != nil {
		info.Name = a.source.Name()
	}

	if src, ok := a.source.(FileSource); ok {
		info.New = src.New
	}

	if f, ok := a.flow.(*errorFlow); ok {
		info.Error = f.message
	}

	if a.doc != nil {
		info.Dirty = a.doc.IsDirty()

		// The selected node is looked up in the tree rather than read off the
		// rows: producing the rows to learn the type of one node would draw
		// the whole document again every time the bar is refreshed, which is
		// the same cost this struct avoids by not carrying a row count.
		if n, ok := domain.Resolve(a.doc.Root(), a.view.Cursor); ok {
			info.Pointer, info.Type = a.view.Cursor.String(), n.Kind().String()
		}
	}

	return info
}
