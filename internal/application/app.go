package application

import (
	"errors"
	"io/fs"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// Deps are the outside capabilities pino needs, as ports. The command line
// builds the adapters and passes them in; nothing below this layer knows
// which implementations they are.
type Deps struct {
	Parser Parser
	Files  FileStore

	// The two ways a document is drawn. Both are built by the caller, as the
	// ports are, so that a test can feed the session rows of its own choosing.
	//
	// They are named fields rather than a map keyed by view: a key left out of
	// a map is a nil call discovered by running the program, whereas a field
	// left out is something the compiler and the exhaustive switch below can
	// both be made to point at.
	JSONView documentview.Renderer
	TreeView documentview.Renderer
}

// Config is what the person running pino chose, as opposed to what pino
// needs in order to run at all.
//
// It is separate from Deps because the two answer different questions. Deps
// says which parser and which file system this session has; Config says how
// the user wants the document written. Putting an indent width among the
// ports would make a policy look like a capability, and every test that needs
// a session would have to supply one.
type Config struct {
	// IndentOverride is one level of indentation, as the command line asked
	// for it. It is a string because that is what a Format holds: four
	// spaces, a tab, or nothing at all.
	IndentOverride string

	// OverrideIndent says the command line asked. Without it there would be
	// no way to tell "--indent 0", which asks for no indentation, from the
	// flag not being given, which asks for the file's own.
	OverrideIndent bool
}

// applyTo is f with the indentation the command line asked for, if it asked.
//
// Everything else about the layout is the file's. The width is the one thing
// a reader may want to impose, because it is the one thing they can see going
// wrong; a file's line endings are not a matter of taste.
func (c Config) applyTo(f domain.Format) domain.Format {
	if c.OverrideIndent {
		f.Indent = c.IndentOverride
	}

	return f
}

// App is the whole state of a pino session.
//
// Everything the user can change lives here rather than in the terminal
// model, so a session is exercised by feeding it Actions and reading back
// its lines and status.
type App struct {
	deps Deps
	cfg  Config

	doc    *Document
	view   ViewState
	source Source

	// flow is what is in progress, and nil when nothing is.
	//
	// The mode is derived from it rather than held beside it. Two fields could
	// disagree — a session in ModeConfirm with nothing to confirm would take
	// keys that led nowhere — and one value cannot.
	//
	// A flow is dropped by assigning nil, never a nil pointer of a flow type:
	// an interface holding one is not nil, and every "is anything in progress"
	// question here is asked by comparing this field against nil.
	flow flow

	// history is every version of the open document. Like the view state it
	// belongs to the document rather than to the session: undoing in one file
	// must not reach into the one opened before it.
	//
	// Its zero value is a history with nothing in it, which is what a session
	// with no document open holds, so undo and redo need no check for one.
	history History

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
func New(d Deps, c Config) *App {
	return &App{
		deps:   d,
		cfg:    c,
		view:   NewViewState(),
		format: c.applyTo(domain.DefaultFormat()),
	}
}

// Open reads and parses the document at path.
//
// The session is left untouched when reading or parsing fails: a document
// that could not be read is reported to the caller, not opened. A path that
// holds nothing is not such a failure — it is where a new document starts.
func (a *App) Open(path string) error {
	read, err := a.read(path, true)
	if err != nil {
		return err
	}

	a.install(read, path)

	// The view state describes the document being looked at, so it does not
	// outlive it: a cursor, a scroll position and a folded set carried over
	// from another file would point at nodes this one does not have.
	a.view = NewViewState()

	return nil
}

// document is a document read from a path, before any of it is installed.
//
// It is returned whole so that reading can fail without leaving the session
// half changed. Every field here replaces one the session already holds, and
// the two callers differ in what they do with them afterwards, not in how
// they are obtained.
type document struct {
	root   domain.Node
	format domain.Format
	meta   Meta
	isNew  bool
}

// read reads and parses the document at path.
//
// The two steps are sequenced here rather than behind a single port, because
// the file store must not know that the bytes are JSON and the parser must
// not know that they came from a file. The layout is detected from the same
// bytes.
//
// allowNew says what a path holding nothing means. Opening one starts an
// empty document, which is how pino is told to write a file that does not
// exist yet; reloading one must not, because a file deleted underneath the
// session would then quietly replace what the reader has been editing with
// an empty object. A link pointing at nothing is neither: the store reports
// it apart from a path that is free, so it stays an error in both.
func (a *App) read(path string, allowNew bool) (document, error) {
	raw, meta, err := a.deps.Files.Read(path)

	switch {
	case allowNew && errors.Is(err, fs.ErrNotExist):
		root, err := domain.NewObject(nil)
		if err != nil {
			// Not reached: an object with no members has no key to be wrong.
			return document{}, err
		}

		// No Meta: what the store recorded is that there was no file, which
		// is what nil says. Saving compares against that rather than skipping
		// the comparison, so a file created meanwhile is not overwritten.
		return document{root: root, format: a.cfg.applyTo(domain.DefaultFormat()), isNew: true}, nil

	case err != nil:
		return document{}, err
	}

	root, err := a.deps.Parser.Parse(raw, domain.StrictJSON)
	if err != nil {
		return document{}, err
	}

	return document{root: root, format: a.cfg.applyTo(domain.DetectFormat(raw)), meta: meta}, nil
}

// install makes a document that has been read the one the session is showing.
//
// It replaces everything that describes the document and touches nothing that
// describes the session: how tall the terminal is, and — for the caller that
// keeps it — which view is drawing. The edit in progress goes, since a
// confirmation or a half typed value left open would be answered against the
// wrong document.
func (a *App) install(read document, path string) {
	a.doc = NewDocument(read.root)
	a.format = read.format
	a.meta = read.meta
	a.source = FileSource{Path: path, New: read.isNew}
	a.flow = nil

	// The history starts again at the document as it was read. The root is
	// where a reader begins, and it resolves in any tree, which is the
	// invariant every later version has to keep.
	a.history = NewHistory(Revision{Root: read.root, Label: "open"})
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
func (a *App) render() []documentview.Line {
	if a.doc == nil {
		return nil
	}

	return a.renderer().Render(a.doc.Root(), a.view.RenderOptions())
}

// renderer is the one drawing the document at the moment.
//
// It is the whole of what switching views does to rendering: the rest of this
// layer asks for rows and is given rows, and never learns which renderer made
// them.
func (a *App) renderer() documentview.Renderer {
	switch a.view.ViewMode {
	case ViewJSON:
		return a.deps.JSONView

	case ViewTree:
		return a.deps.TreeView
	}

	// Not reached: the switch covers every view, and the linter keeps it so.
	return a.deps.JSONView
}

// choose hands a key to whatever is asking.
//
// Which key means what is the flow's own: it drew the choices, so it is the
// one that reads them back. Nothing arrives here when nothing is in progress,
// but an Action driven straight at this layer can, and is answered by doing
// nothing.
func (a *App) choose(key rune) []Effect {
	if a.flow == nil {
		return nil
	}

	return a.flow.choose(a, key)
}

// Mode is what the next key press means.
//
// It follows from whether anything is in progress and how far it has got, so
// there is no state to reset: dropping the flow is returning to normal, and
// the two cannot come apart.
func (a *App) Mode() Mode {
	if a.flow == nil {
		return ModeNormal
	}

	return a.flow.mode()
}

// ViewMode is which of the views is drawing.
//
// Status carries it too, for the bar that names it. This answers the same
// question without looking a node up in the document, which is what laying the
// screen out needs: how the screen is divided depends on the view and not at
// all on what is selected.
func (a *App) ViewMode() ViewMode { return a.view.ViewMode }

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

	case ActionScrollBy:
		a.scrollBy(act.Rows)

	case ActionExpandAll:
		a.view.ExpandAll()
		a.settle(a.render())

	case ActionCollapseAll:
		a.collapseAll()

	case ActionToggleView:
		a.toggleView()

	case ActionEdit:
		return a.edit()

	case ActionRenameKey:
		return a.renameKey()

	case ActionAddChild:
		return a.addChild()

	case ActionAddSibling:
		return a.addSibling()

	case ActionDelete:
		a.deleteSelected()

	case ActionChangeType:
		a.changeType()

	// The four below answer a prompt. They do nothing when no edit is in
	// progress: they cannot arrive then, since nothing is on screen to send
	// them, but an Action that could not be delivered is a better answer than
	// one that reaches into a flow that is not there.
	case ActionPromptChange:
		a.validate(act.Text)

	case ActionPromptSubmit:
		a.submit(act.Text)

	case ActionPromptChoose:
		return a.choose(act.Key)

	case ActionSave:
		a.save(false)

	case ActionCancel:
		a.cancel()

	case ActionUndo:
		a.restore(a.history.Undo())

	case ActionRedo:
		a.restore(a.history.Redo())

	case ActionResize:
		// A window of no rows is not a window; asking for one is answered by
		// scrolling nowhere rather than by arithmetic on a negative height.
		a.height = max(act.Height, 0)
		a.settle(a.render())
	}

	return nil
}

// restore makes a version current: the tree it holds, and a look at the place
// the change being toggled happened.
//
// It takes what History returned rather than a Revision alone, so that undo
// and redo differ by the one word that names their direction. Nothing happens
// when there is nowhere to go, which is also how a session with no document
// open answers: its history is empty, so both report false and this never
// reaches a nil document.
//
// There is no inverse of anything here. Every edit pino can make comes back to
// swapping one immutable root for another, which is why undo cannot corrupt a
// document by undoing something wrongly.
//
// Folds are not restored, only dropped where the tree no longer has them: what
// is folded is how the document is being looked at, and this restores what the
// document contains.
func (a *App) restore(rev Revision, at domain.Path, ok bool) {
	if !ok {
		return
	}

	a.doc.Replace(rev.Root)
	// Answers gathered against another root cannot safely be applied to this
	// one. The terminal does not route undo or redo through a prompt, but the
	// application keeps this invariant even when actions are driven directly.
	a.flow = nil
	a.view.Retain(rev.Root)

	// Undoing an insertion takes away the node it selected, so the place the
	// change happened is not always in the tree coming back. The nearest thing
	// left to it is; being off by a container beats being sent to the top of
	// the document. Whether the result is on screen is settled next.
	a.view.Cursor = nearest(rev.Root, at)

	a.settle(a.render())
}

// nearest is the deepest part of p that root still has, which is p itself
// whenever it resolves.
//
// The walk always ends: the root is its own parent and resolves in any tree.
func nearest(root domain.Node, p domain.Path) domain.Path {
	for ; !p.IsRoot(); p = p.Parent() {
		if _, ok := domain.Resolve(root, p); ok {
			return p
		}
	}

	return p
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
func (a *App) settle(lines []documentview.Line) {
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
func (a *App) moveBy(step func(lines []documentview.Line, from int) int) {
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
func (a *App) moveTo(pick func(lines []documentview.Line) int) {
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

// scrollBy moves the window by rows, positive downwards, taking the selection
// with it only as far as keeping it on screen requires.
//
// It is the one movement that starts with the window rather than with the
// selection, which is what turning a wheel asks for. The selection is still
// not left behind: the status bar names the node it is on, and naming one
// nobody can see says less than nudging the selection does.
func (a *App) scrollBy(rows int) {
	lines := a.render()

	// Without a window there is nothing to scroll and no edge to be pushed
	// over, so the selection is left exactly where it is.
	if len(lines) == 0 || a.height <= 0 {
		a.settle(lines)

		return
	}

	// No cursor row is passed: this is asking for the offset to be brought
	// into range and nothing more, the same way half a screen does.
	scroll := clampScroll(a.view.Scroll+rows, -1, a.height, len(lines))
	a.view.Scroll = scroll

	if from := visibleRow(lines, a.view.Cursor); from >= 0 {
		if to := intoWindow(lines, from, scroll, a.height); to >= 0 {
			a.view.Cursor = lines[to].Path
		}
	}

	a.settle(lines)
}

// toggleView draws the document the other way, leaving the reader looking at
// the same node from the same place on the screen.
//
// The selection needs nothing done to it. It is held as a path, and both
// renderers draw a row for the same nodes in the same order, so the node it
// names is on screen in the view being switched to.
//
// The window does need something done to it. The offset is a row number, and
// neither the number of rows nor the row a given node sits on survives the
// switch: a node on row 120 of the JSON view may be on row 60 of the tree.
// Left alone, the window would show somewhere else entirely.
//
// What is carried across is where the cursor sat within the window rather than
// where the window sat in the document. Letting settle place it would put the
// cursor at the top of the screen whenever the rows below it grew fewer, and a
// line jumping from the middle of the screen to the top is a larger change than
// "show me this the other way" asks for.
//
// It is carried as far as the other view can take it, which near the end of a
// document is not always the whole way: a view with fewer rows cannot put a
// node close to its end as far down the screen. settle brings the offset back
// into range, and the switch after that reads the corrected one, so a round
// trip taken near an end returns to the same node with the window a little
// higher or lower than it left. The shift happens once and then stops, since
// the second switch has nothing left to correct. Undoing it would mean holding
// on to an offset from before it was corrected, which every other action would
// then have to know to throw away: a hidden second thing to settle, for a row
// or two at the ends of a document.
//
// The document is laid out twice, once each side of the switch, because the
// state of the window before can only be read from the rows before. Tab is not
// a key held down, so this is not where memoising would first be wanted.
func (a *App) toggleView() {
	before := a.render()

	// How far down the window the cursor was. A cursor off the screen, or a
	// session with nothing open, leaves the offset at the top row.
	offset := 0
	if row := visibleRow(before, a.view.Cursor); row >= 0 {
		offset = min(max(row-a.view.Scroll, 0), max(a.height-1, 0))
	}

	a.view.ViewMode = a.view.ViewMode.Next()

	lines := a.render()
	if row := visibleRow(lines, a.view.Cursor); row >= 0 {
		a.view.Scroll = row - offset
	}

	// settle has the last word, so an offset that cannot be honoured at the
	// ends of a shorter document is brought back into range here rather than
	// guarded against above.
	a.settle(lines)
}

// collapseAll folds the document down to an overview of its shape.
//
// It is the one movement needing the tree rather than the rows: what is folded
// away cannot be found among what is drawn, so the containers to fold are
// collected by walking the document itself. Reaching for the tree is what
// raises the question of whether there is one, which is why this asks and the
// rest of the movements do not. Editing reaches for it too, and asks through
// selected.
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

	if lines[from].Kind == documentview.LineOpen && !lines[from].Path.IsRoot() {
		if a.view.Collapse(lines[from].Path) {
			a.settle(a.render())

			return
		}
	} else if to := parentRow(lines, from); to >= 0 {
		a.view.Cursor = lines[to].Path
	}

	a.settle(lines)
}
