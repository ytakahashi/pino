package application

import "github.com/ytakahashi/pino/internal/domain"

// ViewMode selects which renderer draws the document.
type ViewMode uint8

const (
	ViewJSON ViewMode = iota
	ViewTree
)

// String returns the label shown in the status bar.
func (v ViewMode) String() string {
	switch v {
	case ViewJSON:
		return "JSON"
	case ViewTree:
		return "TREE"
	default:
		return "UNKNOWN"
	}
}

// ViewState is how the document is being looked at, as opposed to what the
// document is.
//
// It survives a view switch untouched: the folded set and the cursor are
// shared by both renderers, so toggling between them does not move the
// selection or re-expand what the user has folded away.
type ViewState struct {
	// Collapsed holds the JSON Pointer of every folded node. Empty means
	// everything is expanded, which is where a document starts.
	Collapsed map[string]struct{}

	Cursor   domain.Path
	ViewMode ViewMode
	Scroll   int
}

// NewViewState is a fully expanded document seen in the JSON view, with the
// cursor at the root.
func NewViewState() ViewState {
	return ViewState{Collapsed: make(map[string]struct{})}
}

// RenderOptions is the part of the view state a renderer needs.
//
// The map is handed over rather than copied: a renderer only reads it, and
// rendering happens on every redraw.
func (v ViewState) RenderOptions() RenderOptions {
	return RenderOptions{Collapsed: v.Collapsed}
}
