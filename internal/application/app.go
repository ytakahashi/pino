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

	// height is how many rows the document has to itself, as reported by
	// whoever is drawing it.
	//
	// It is not part of the view state, which describes the document being
	// looked at and is discarded when another is opened. How tall the terminal
	// is outlives any document, so keeping it here saves the view state from
	// growing an exception to that rule.
	height int
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

// Frame is the session as it should be drawn. It holds no rows when nothing is
// open.
//
// It only reads: the cursor is looked up, never corrected. Drawing happens on
// every message Bubble Tea delivers, and a query that quietly repaired state
// would hide the fact that something had left it broken. Correcting is what
// settle does, once per action.
func (a *App) Frame() Frame {
	lines := a.render()

	return Frame{
		Lines:  lines,
		Cursor: indexOf(lines, a.view.Cursor),
		Scroll: a.view.Scroll,
	}
}

// render lays the open document out, or produces nothing when none is open.
//
// Answering with no rows rather than refusing is what lets the rest of the
// layer be written without a branch for "nothing is open": moving finds no
// cursor row, settling finds nothing to settle, and the window stays at the
// top.
//
// The document is laid out again on every call. Bubble Tea draws for every
// message it receives, so this will want memoising on the root and the render
// options; until it measurably hurts, rendering afresh keeps the rows
// impossible to get out of step with the document.
func (a *App) render() []Line {
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
		Mode:     a.mode,
		ViewMode: a.view.ViewMode,
		Indent:   a.format.Indent,
	}

	if a.source != nil {
		info.Name = a.source.Name()
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

// Do applies an Action and returns the work the presentation layer has to
// carry out. An Action this session has nothing to do with yields no effect.
func (a *App) Do(act Action) []Effect {
	switch act := act.(type) {
	case ActionQuit:
		return []Effect{EffectQuit{}}

	case ActionMoveNext:
		a.moveBy(nextRow)

	case ActionMovePrev:
		a.moveBy(prevRow)

	case ActionMoveIn:
		a.moveIn()

	case ActionMoveOut:
		a.moveOut()

	case ActionMoveFirst:
		a.moveTo(firstRow)

	case ActionMoveLast:
		a.moveTo(lastRow)

	case ActionScrollHalfDown:
		a.scrollHalf(+1)

	case ActionScrollHalfUp:
		a.scrollHalf(-1)

	case ActionExpandAll:
		a.view.ExpandAll()
		a.settle(a.render())

	case ActionCollapseAll:
		a.collapseAll()

	case ActionResize:
		// A window of no rows is not a window; asking for one is answered by
		// scrolling nowhere rather than by arithmetic on a negative height.
		a.height = max(act.Height, 0)
		a.settle(a.render())
	}

	return nil
}

// settle puts the cursor and the window back into agreement with lines.
//
// Every action ends here, which is what makes "the cursor is on screen" true
// after all of them rather than after each having remembered to arrange it.
// The cursor is written back rather than merely drawn elsewhere: a path
// pointing at a node no longer on screen would go on naming that node in the
// status bar, and would decide where the next keystroke moves from.
//
// lines is a parameter rather than something produced here, so that the
// actions leaving the rows alone say so by handing over the ones they already
// have. Only the ones that change which rows exist lay the document out again.
func (a *App) settle(lines []Line) {
	row := visibleRow(lines, a.view.Cursor)
	if row < 0 {
		row = firstRow(lines)
	}

	if row >= 0 {
		a.view.Cursor = lines[row].Path
	}

	scroll := a.view.Scroll

	// The rows closing whatever is still open come after the last node, and
	// the cursor never lands on them. Standing on the last node with those
	// rows just off the bottom looks exactly like standing in the middle of a
	// document, so the end of one is shown whole rather than to the least
	// extent the cursor requires.
	if row >= 0 && row == lastRow(lines) {
		scroll = max(scroll, len(lines)-a.height)
	}

	a.view.Scroll = clampScroll(scroll, row, a.height, len(lines))
}

// moveBy selects the row step leads to, staying put when it leads nowhere.
//
// The document is laid out once: which rows exist does not depend on where the
// cursor is, so the same rows answer both where to go and what to settle
// against.
func (a *App) moveBy(step func(lines []Line, from int) int) {
	lines := a.render()

	if from := visibleRow(lines, a.view.Cursor); from >= 0 {
		if to := step(lines, from); to >= 0 {
			a.view.Cursor = lines[to].Path
		}
	}

	a.settle(lines)
}

// moveTo selects whichever row pick chooses.
//
// Unlike moveBy this needs no row to start from: where these actions go does
// not depend on where they begin, so they still work when the cursor has been
// left pointing at something no longer drawn.
func (a *App) moveTo(pick func(lines []Line) int) {
	lines := a.render()

	if to := pick(lines); to >= 0 {
		a.view.Cursor = lines[to].Path
	}

	a.settle(lines)
}

// scrollHalf moves the window and the cursor half a screen, downwards for a
// positive dir and upwards for a negative one.
//
// The window is moved here rather than left for settle to follow: minimal
// scrolling alone would pin the cursor to the edge of the screen while the
// text barely moved, which is not what reading half a page at a time looks
// like. settle still runs afterwards and has the last word, which is what
// keeps the two in agreement at the ends of a document.
//
// The cursor travels a count of rows rather than a count of nodes. Stepping
// node by node would cover more ground wherever closing rows were skipped, and
// the cursor would drift down the screen over several presses; counting rows
// keeps it where it was. Landing on a row it cannot occupy is answered by the
// nearest one it can.
func (a *App) scrollHalf(dir int) {
	lines := a.render()
	if len(lines) == 0 {
		a.settle(lines)

		return
	}

	// Half a window, and never less than a row: a window too small to halve
	// still has to move, and a terminal that has not yet reported its size is
	// no reason to refuse.
	step := max(a.height/2, 1) * dir

	if from := visibleRow(lines, a.view.Cursor); from >= 0 {
		if to := nearestRow(lines, from+step, dir); to >= 0 {
			a.view.Cursor = lines[to].Path
		}
	}

	// Bounding the window without regard to the cursor, which settle then
	// takes into account: passing no cursor row is how this asks for the
	// offset to be brought into range and nothing more.
	a.view.Scroll = clampScroll(a.view.Scroll+step, -1, a.height, len(lines))

	a.settle(lines)
}

// collapseAll folds the document down to an overview of its shape.
//
// It is the one action needing the tree rather than the rows: what is folded
// away cannot be found among what is drawn, so the containers to fold are
// collected by walking the document itself. That is also why this is the one
// place left checking whether a document is open at all.
func (a *App) collapseAll() {
	if a.doc == nil {
		return
	}

	a.view.CollapseAll(a.doc.Root())
	a.settle(a.render())
}

// moveIn unfolds the selected container, or selects the first thing inside one
// that is already open.
//
// Unfolding leaves the cursor where it is, as vim does: what was hidden
// appears, and stepping into it is the next keystroke. Nothing happens on a
// value with no children, in the same way that there is nothing to open.
func (a *App) moveIn() {
	lines := a.render()

	from := visibleRow(lines, a.view.Cursor)
	if from < 0 {
		a.settle(lines)

		return
	}

	if lines[from].Collapsed {
		if a.view.Expand(lines[from].Path) {
			// The rows that were hidden are back, so the ones in hand are
			// stale.
			a.settle(a.render())

			return
		}
	} else if to := firstChildRow(lines, from); to >= 0 {
		a.view.Cursor = lines[to].Path
	}

	a.settle(lines)
}

// moveOut folds the selected container away, or selects what holds it.
//
// An open container folds; anything else, a folded container included, steps
// out to its parent. That is what gives one key both meanings without a mode:
// pressing it repeatedly walks out of a document, folding each level on the
// way only while standing on something open.
//
// The root does neither: it has no parent, so it is where walking out ends,
// and folding it would leave a screen holding "{…}" and no document at all.
func (a *App) moveOut() {
	lines := a.render()

	from := visibleRow(lines, a.view.Cursor)
	if from < 0 {
		a.settle(lines)

		return
	}

	if lines[from].Kind == LineOpen && !lines[from].Path.IsRoot() {
		if a.view.Collapse(lines[from].Path) {
			a.settle(a.render())

			return
		}
	} else if to := parentRow(lines, from); to >= 0 {
		a.view.Cursor = lines[to].Path
	}

	a.settle(lines)
}
