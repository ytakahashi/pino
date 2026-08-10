package presentation

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
)

// Model is what Bubble Tea runs pino as.
//
// Beyond the size of the terminal it holds no state: the document, the mode
// and the view state all live in the application, so what is drawn is a
// reading of the session rather than a copy of it that could drift out of
// step.
type Model struct {
	app   *application.App
	theme Theme

	width  int
	height int

	// pending is a prefix key waiting for the one that completes it. It is
	// the only thing here that outlives a single message and is not the
	// session's, because a half-typed sequence is not a fact about the
	// document.
	pending Pending

	// mouse is whether the terminal is asked to report the wheel.
	//
	// Asking costs the terminal's own text selection: there is no mode that
	// reports the wheel alone, so clicks and drags are captured too and
	// dragging no longer selects text to copy. That is a real loss for anyone
	// reading JSON over ssh, which is why the choice is a value here rather
	// than something written into the frame: turning it off is one field, and
	// an option to would set it where the program is assembled.
	mouse bool
}

// NewModel puts a session on the terminal.
//
// The theme is a parameter rather than a package default so that the colours
// are settled where the rest of the program is assembled, which is where an
// option to choose one would arrive.
func NewModel(app *application.App, theme Theme) Model {
	return Model{app: app, theme: theme, mouse: true}
}

// Init has nothing to start. The document is opened before the program runs,
// so that a file which cannot be read is reported on the terminal pino was
// launched from rather than on a screen it has already taken over.
func (m Model) Init() tea.Cmd { return nil }

// Update answers a message with the next model.
//
// A key press is resolved to an Action, handed to the application, and what
// comes back is a list of effects to carry out. Nothing here decides what a
// key does; this is only the place that knows a key was pressed at all.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

		// The application follows the cursor with the window, so it has to be
		// told how big that window is. What it is told is the room left for
		// the document rather than the height of the terminal: taking the
		// status bar off the top of it, and later an inspector, is a decision
		// about laying out a screen and stays on this side of the boundary.
		return m, m.dispatch(m.app.Do(application.ActionResize{Height: m.bodyHeight()}))

	case tea.KeyPressMsg:
		act, pending := Resolve(msg, m.app.Mode(), m.pending)
		m.pending = pending

		if act == nil {
			return m, nil
		}

		return m, m.dispatch(m.app.Do(act))

	case tea.MouseWheelMsg:
		rows, ok := wheelDistance(msg)
		if !ok {
			return m, nil
		}

		return m, m.dispatch(m.app.Do(application.ActionScrollBy{Rows: rows}))
	}

	return m, nil
}

// wheelRows is how far one turn of the wheel reaches. Three is what terminals
// and the programs drawn in them have settled on.
const wheelRows = 3

// wheelDistance is how far a turn of the wheel scrolls, and whether it scrolls
// at all.
//
// The horizontal wheel is left unbound: a row of a document is cut to the
// width of the screen rather than scrolled sideways, so there is nothing for
// it to move.
func wheelDistance(msg tea.MouseWheelMsg) (int, bool) {
	switch msg.Button {
	case tea.MouseWheelDown:
		return wheelRows, true

	case tea.MouseWheelUp:
		return -wheelRows, true
	}

	return 0, false
}

// dispatch carries out what the application asked for.
//
// An effect this does not recognise is dropped rather than reported: effects
// arrive together with the code that produces them, so an unknown one is a
// mistake within pino, and losing it is a better answer than tearing the
// terminal down in front of the person using it.
func (m Model) dispatch(effects []application.Effect) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(effects))

	for _, e := range effects {
		switch e.(type) {
		case application.EffectQuit:
			cmds = append(cmds, tea.Quit)
		}
	}

	return tea.Batch(cmds...)
}

// View draws the document with the status bar along the bottom.
func (m Model) View() tea.View {
	// Nothing can be laid out before the terminal has said how big it is.
	// The size arrives as a message, so the first frame is empty and is
	// replaced as soon as it comes.
	if m.width <= 0 || m.height <= 0 {
		return fullScreen("", m.mouse)
	}

	frame := m.app.Frame()
	info := m.app.Status()
	body := m.layout().BodyWidth

	rows := make([]string, 0, m.height)

	window, start := m.visible(frame)

	for i, l := range window {
		selected := start+i == frame.Cursor
		row := clip(m.theme.RenderLine(l, indentFor(info), selected), body)

		// The band behind the selected row runs to the far side of the
		// document, not to the far side of the text. Where a row happens to
		// stop has nothing to do with which row is selected, so a highlight
		// stopping there reads as ragged rather than as a row being pointed
		// at. It stops at the document's own edge so that it cannot reach into
		// the inspector standing beside it.
		if selected {
			row += m.theme.RenderCursorFill(body - ansi.StringWidth(row))
		}

		rows = append(rows, row)
	}

	// Blank rows hold the bar at the bottom when the document is shorter than
	// the screen.
	for len(rows) < m.bodyHeight() {
		rows = append(rows, "")
	}

	rows = append(rows, m.theme.RenderStatusBar(info, len(frame.Lines), m.pending, m.width))

	return fullScreen(strings.Join(rows, "\n"), m.mouse)
}

// fullScreen is a frame drawn on a screen of pino's own.
//
// The document is laid out against the height of the terminal, with the status
// bar held at the bottom by blank rows, which is only coherent if pino owns
// every row. Drawing that into the shell's scrollback would instead leave the
// blank rows and every intermediate frame behind it. The alternate screen also
// gives back what was on the terminal when pino quits, so that opening a file
// to look at it does not cost the person the output they had.
// The mouse is asked for here too, since the terminal is told what to report
// by each frame rather than once at the start.
func fullScreen(content string, mouse bool) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true

	if mouse {
		// The narrowest mode there is. It still captures clicks and drags,
		// there being none that reports the wheel alone.
		v.MouseMode = tea.MouseModeCellMotion
	}

	return v
}

// layout is how this model's screen is divided.
func (m Model) layout() layout {
	return layoutFor(m.width, m.height, m.app.ViewMode())
}

// bodyHeight is how many rows the document has to itself.
//
// The status bar takes the last one. The inspector standing under the tree
// will take more, and this becomes the layout's BodyHeight when it does. Until
// then it stays the height of the terminal less the bar, because the number is
// also what the session is told: reserving rows on screen without saying so
// would leave the session scrolling for a window taller than the one being
// drawn, and the cursor could sit below what anyone can see.
func (m Model) bodyHeight() int {
	return max(m.height-statusBarRows, 0)
}

// treeIndent is one level of the tree, which is also the width of a marker:
// a child's name therefore sits under its parent's.
const treeIndent = "  "

// indentFor is one level of indentation in the view being drawn.
//
// The JSON view uses the document's own, because the whitespace it draws is
// the whitespace that will be saved: a file written with tabs is shown with
// tabs. Nothing the tree view draws is ever saved, so the same reason does not
// reach it, and following the document there would do harm instead — a
// tab-indented file would run the tree off the right of the screen, and one
// written with eight spaces would spend forty columns on a depth of five.
// Overlooking a document's shape is what the tree view is for.
//
// The status bar goes on reporting the document's own value in either view.
// What it says is what saving will do, not what the screen is doing.
func indentFor(info application.StatusInfo) string {
	switch info.ViewMode {
	case application.ViewTree:
		return treeIndent

	case application.ViewJSON:
	}

	return info.Indent
}

// visible is the part of the document that fits on the screen, along with the
// row number it starts at.
//
// Where it starts is the application's to decide, since that is what follows
// the cursor about. The offset comes back out because a row has to be told
// whether it is the selected one, and what it is compared against is a
// position in the whole document rather than in the part on screen.
//
// The offset is brought into range rather than trusted: it was worked out
// against a height this layer reported, and a frame drawn between the two
// would otherwise index outside the document.
func (m Model) visible(frame application.Frame) ([]application.Line, int) {
	start := min(max(frame.Scroll, 0), len(frame.Lines))
	end := min(start+m.bodyHeight(), len(frame.Lines))

	return frame.Lines[start:end], start
}

// clip cuts a row to the width the document has, which is the terminal's less
// whatever stands beside it.
//
// Rows are never wrapped: one line of the document is one row on the screen,
// which is what allows a line to be found by its position. A line too long to
// fit is therefore cut off. Shortening long values, which is the usual reason
// for one, belongs to the renderer rather than to the display, since only the
// renderer can leave a mark saying that something was left out.
func clip(row string, width int) string {
	return ansi.Truncate(row, width, "")
}
