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

	rows := make([]string, 0, m.height)

	window, start := m.visible(frame)

	for i, l := range window {
		// The indentation comes from the open document rather than from the
		// theme, so that what is drawn and what the status bar reports are
		// the same value.
		selected := start+i == frame.Cursor
		row := m.clip(m.theme.RenderLine(l, info.Indent, selected))

		// The band behind the selected row runs to the edge of the screen.
		// Where the text stops has nothing to do with which row is selected,
		// so letting the highlight stop there would make it look ragged.
		if selected {
			row += m.theme.RenderCursorFill(m.width - ansi.StringWidth(row))
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

// bodyHeight is how many rows the document has to itself.
//
// The status bar takes the last one. The tree view's inspector will take
// more, which is why the arithmetic sits here rather than spread through the
// drawing.
func (m Model) bodyHeight() int {
	return max(m.height-1, 0)
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

// clip cuts a row to the width of the terminal.
//
// Rows are never wrapped: one line of the document is one row on the screen,
// which is what allows a line to be found by its position. A line too long to
// fit is therefore cut off. Shortening long values, which is the usual reason
// for one, belongs to the renderer rather than to the display, since only the
// renderer can leave a mark saying that something was left out.
func (m Model) clip(row string) string {
	return ansi.Truncate(row, m.width, "")
}
