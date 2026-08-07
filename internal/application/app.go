package application

import "github.com/ytakahashi/pino/internal/domain"

// Deps are the outside capabilities pino needs, as ports. The command line
// builds the adapters and passes them in; nothing below this layer knows
// which implementations they are.
type Deps struct {
	Parser   Parser
	Files    FileStore
	Renderer Renderer
}

// App is the whole state of a pino session.
//
// Everything the user can change lives here rather than in the terminal
// model, so a session is exercised by feeding it Actions and reading back
// its lines and status.
type App struct {
	deps Deps

	doc    *Document
	view   ViewState
	mode   Mode
	source Source

	// format is the layout the document is written back with, taken from the
	// file it was read from so that saving does not reformat lines the user
	// never touched.
	format domain.Format

	// meta is what the file store handed over when reading, kept to give
	// back before writing. Its contents are never read here.
	meta Meta
}

// New starts a session with no document open.
func New(d Deps) *App {
	return &App{
		deps:   d,
		view:   NewViewState(),
		mode:   ModeNormal,
		format: domain.DefaultFormat(),
	}
}

// Open reads and parses the document at path.
//
// The two steps are sequenced here rather than behind a single port, because
// the file store must not know that the bytes are JSON and the parser must
// not know that they came from a file. The layout is detected from the same
// bytes.
//
// The session is left untouched when either step fails: a document that
// could not be parsed is reported to the caller, not opened.
func (a *App) Open(path string) error {
	raw, meta, err := a.deps.Files.Read(path)
	if err != nil {
		return err
	}

	root, err := a.deps.Parser.Parse(raw, domain.StrictJSON)
	if err != nil {
		return err
	}

	a.doc = NewDocument(root)
	a.format = domain.DetectFormat(raw)
	a.meta = meta
	a.source = FileSource{Path: path}

	// The view state describes the document being looked at, so it does not
	// outlive it: a cursor, a scroll position and a folded set carried over
	// from another file would point at nodes this one does not have. The
	// mode is reset for the same reason, since a confirmation or an edit
	// left open would be answered against the wrong document.
	a.view = NewViewState()
	a.mode = ModeNormal

	return nil
}

// Lines renders the open document. It returns nil when nothing is open.
//
// The document is rendered again on every call. Bubble Tea calls View for
// every message it receives, so this will want memoising on the root and the
// render options; until it measurably hurts, re-rendering keeps the lines
// impossible to get out of step with the document.
func (a *App) Lines() []Line {
	if a.doc == nil {
		return nil
	}

	return a.deps.Renderer.Render(a.doc.Root(), a.view.RenderOptions())
}

// Mode is what the next key press means.
func (a *App) Mode() Mode { return a.mode }

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
}

// Status describes the session for the status bar.
func (a *App) Status() StatusInfo {
	info := StatusInfo{
		Mode:     a.mode,
		ViewMode: a.view.ViewMode,
		Indent:   a.format.Indent,
	}

	if a.source != nil {
		info.Name = a.source.Name()
	}

	if a.doc != nil {
		info.Dirty = a.doc.IsDirty()
	}

	return info
}

// Do applies an Action and returns the work the presentation layer has to
// carry out. An Action this session has nothing to do with yields no effect.
func (a *App) Do(act Action) []Effect {
	switch act.(type) {
	case ActionQuit:
		return []Effect{EffectQuit{}}
	}

	return nil
}
