package presentation

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
	"github.com/ytakahashi/pino/internal/application/documentview"
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

	// editor is the box an answer is being typed into, and nothing at all when
	// no answer is being typed.
	//
	// The text is here rather than in the session because the widget holding it
	// belongs to the terminal, and a second copy below could differ from what
	// is on screen. What the session is told is what has been typed, each time
	// it changes; what it keeps is why the answer cannot be taken yet.
	editor editor

	// reported is the last body height the session was told about.
	//
	// It is here rather than asked of the session because what matters is that
	// the number is sent when it changes, not when a particular message
	// arrives. A resized terminal is one cause of a change; a view that
	// brought an inspector with it is another, and that one comes of a key
	// press.
	reported int

	// mouse is whether the terminal is asked to report the wheel.
	//
	// Asking costs the terminal's own text selection: there is no mode that
	// reports the wheel alone, so clicks and drags are captured too and
	// dragging no longer selects text to copy. That is a real loss for anyone
	// reading JSON over ssh, which is why the choice is a value here rather
	// than something written into the frame: an option chooses its initial
	// value, and the terminal key can reverse that choice while pino is open.
	mouse bool
}

// ModelConfig is the terminal behaviour chosen where the program is assembled.
//
// DisableMouse is negative so that the zero value preserves pino's default:
// the terminal reports the wheel unless the reader explicitly gives text
// selection priority instead.
type ModelConfig struct {
	DisableMouse bool
}

// NewModel puts a session on the terminal.
//
// The theme is a parameter rather than a package default so that the colours
// are settled where the rest of the program is assembled, which is where an
// option to choose one would arrive.
func NewModel(app *application.App, theme Theme, cfg ModelConfig) Model {
	return Model{app: app, theme: theme, mouse: !cfg.DisableMouse}
}

// Init has nothing to start. The document is opened before the program runs,
// so that a file which cannot be read is reported on the terminal pino was
// launched from rather than on a screen it has already taken over.
func (m Model) Init() tea.Cmd { return nil }

// Update answers a message with the next model.
//
// A key press is resolved to an application Action or a terminal request. An
// Action is handed to the application and its effects are carried out; a
// terminal request changes only the presentation state held here. The keymap,
// rather than this function, still decides what the key asks for.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.editor = m.editor.SetWidth(inputWidth(m.app.Prompt(), m.width))

		return m.syncHeight(nil)

	case tea.KeyPressMsg:
		return m.key(msg)

	case tea.PasteMsg:
		// What the terminal pastes arrives whole rather than as the keys it
		// stands for, so it has a branch of its own. It goes to the box or
		// nowhere: there is nothing in a document to paste into until pino can
		// copy a subtree, and a stray paste is better ignored than turned into
		// however many movements its characters happen to name.
		if m.app.Prompt().Kind != application.PromptText {
			return m, nil
		}

		m.editor = m.editor.Update(msg)

		return m.act(application.ActionPromptChange{Text: m.editor.Value()})

	case tea.MouseWheelMsg:
		// The wheel scrolls the document, and help is the one screen drawn
		// without one on it. A turn taken there would move the offset and
		// carry the selection along with it, so the reader would close help
		// onto a screen they had not moved. A question is different: the
		// document is still behind it, and scrolling to look at more of it is
		// part of answering.
		//
		// A wheel event can already be queued when the frame that disables
		// reporting reaches the terminal. Once reporting is off, pino ignores
		// input it is no longer asking the terminal to send.
		if !m.mouse || m.app.Mode() == application.ModeHelp {
			return m, nil
		}

		rows, ok := wheelDistance(msg)
		if !ok {
			return m, nil
		}

		return m.act(application.ActionScrollBy{Rows: rows})
	}

	return m, nil
}

// key routes a key press to whatever is waiting for one.
//
// There are three destinations, and what is on screen decides between them: a
// box being typed into takes the key itself, a list of choices resolves it
// against what is on offer, and everything else goes through the key table.
//
// Asking the prompt rather than the mode is what keeps the two from having to
// agree. A prompt is drawn from the mode's own state, so "there is a box on
// screen" and "the key belongs to the box" are one fact read once.
func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// The terminal's way out is bound before anything else, so that no prompt
	// can become a dead end. The key table binds it too, for the presses that
	// reach it; this is the copy a text box cannot swallow.
	if msg.String() == "ctrl+c" {
		return m.act(application.ActionQuit{})
	}

	switch p := m.app.Prompt(); p.Kind {
	case application.PromptText:
		return m.typed(msg)

	case application.PromptChoice:
		return m.act(ResolveChoice(msg, p))

	case application.PromptNone:
		act, terminal, pending := Resolve(msg, m.app.Mode(), m.pending)
		m.pending = pending

		switch terminal {
		case TerminalToggleMouse:
			m.mouse = !m.mouse

		case TerminalNone:
		}

		return m.act(act)
	}

	return m, nil
}

// typed hands a key press to the box, after taking the ones that are the
// prompt's rather than the box's.
//
// Those three cannot be left to the widget: Enter and Esc would be typed into
// the text, and a newline has to be asked for by a key the terminal can tell
// from Enter. Everything else is the widget's, which is why the key table is
// not consulted here at all — a table that knew about text would have to know
// what had been typed so far.
func (m Model) typed(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return m.act(application.ActionPromptSubmit{Text: m.editor.Value()})

	case "esc":
		return m.act(application.ActionCancel{})

	case "ctrl+j":
		// LF rather than CR, which is what lets a terminal tell this from
		// Enter. A box that cannot hold a newline is left as it was.
		m.editor = m.editor.InsertNewline()

	default:
		m.editor = m.editor.Update(msg)
	}

	// What has been typed is reported on every key, so that an answer which
	// cannot be committed says so while it is being typed rather than when
	// Enter is pressed.
	return m.act(application.ActionPromptChange{Text: m.editor.Value()})
}

// act carries an Action out and brings the screen back into agreement with
// what it did. An Action nothing was asked for leaves everything as it was.
func (m Model) act(a application.Action) (tea.Model, tea.Cmd) {
	if a == nil {
		return m, nil
	}

	m, cmd := m.dispatch(m.app.Do(a))

	// The box belongs to the prompt that asked for it: when the session is no
	// longer waiting to be typed at, what was typed goes with the question.
	// Dropping it here rather than at each of the ways an edit can end is what
	// keeps a box from outliving one of them.
	if m.app.Prompt().Kind != application.PromptText {
		m.editor = editor{}
	}

	return m.syncHeight(cmd)
}

// syncHeight tells the session how much room the document has, whenever that
// changes, and carries cmd along with whatever saying so produced.
//
// The application follows the cursor with the window, so it has to be told how
// big that window is. What it is told is the room left for the document rather
// than the height of the terminal: taking off the status bar, the inspector
// standing under the tree and the band a question is being asked in is a
// decision about laying out a screen and stays on this side of the boundary.
//
// Every branch of Update ends here, including the ones that cannot change the
// height today. What may change it is what the session is allowed to do, and
// that grows; a branch that reported only because someone remembered to add it
// would be the one to go quiet later.
//
// The report is an ActionResize whatever brought it about. To the session this
// says "the document has this many rows now", and why is not its business —
// the same generalisation that made Height the room for the document rather
// than the height of the terminal.
func (m Model) syncHeight(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	h := m.layout().BodyHeight
	if h == m.reported {
		return m, cmd
	}

	m.reported = h

	m, resized := m.dispatch(m.app.Do(application.ActionResize{Height: h}))

	return m, tea.Batch(cmd, resized)
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
//
// It returns a model as well as a command because an effect may be something
// this layer has to hold rather than something to run: a box to type into is
// built here and stays until the question is answered.
func (m Model) dispatch(effects []application.Effect) (Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0, len(effects))

	for _, e := range effects {
		switch e := e.(type) {
		case application.EffectQuit:
			cmds = append(cmds, tea.Quit)

		case application.EffectBeginInput:
			// The prompt has already been asked for its shape, so the box is
			// built to the room that shape leaves it.
			//
			// A box that did not take the value whole ends the edit instead of
			// starting it: Enter reads the answer out of the box, so an edit
			// begun on less than the value would commit a value nobody typed.
			// Nothing is expected to reach this — the widget is given limits
			// past anything a terminal can draw — and a session that did would
			// rather refuse to edit than quietly rewrite.
			if box, ok := newEditor(m.theme, e, inputWidth(m.app.Prompt(), m.width)); ok {
				m.editor = box
			} else {
				m.app.Do(application.ActionCancel{})
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// View draws the document with the status bar along the bottom.
func (m Model) View() tea.View {
	// Nothing can be laid out before the terminal has said how big it is.
	// The size arrives as a message, so the first frame is empty and is
	// replaced as soon as it comes.
	if m.width <= 0 || m.height <= 0 {
		return fullScreen("", m.mouse)
	}

	// Below the size pino draws in, the screen says so instead. It is not a
	// mode: the session behind it is untouched, and widening the terminal
	// brings the document back exactly as it was left.
	l := m.layout()
	if l.TooSmall {
		return fullScreen(m.theme.RenderTooSmall(m.width, m.height), m.mouse)
	}

	frame := m.app.Frame()
	info := m.app.Status()
	bar := barState{Lines: len(frame.Lines), Pending: m.pending, Mouse: m.mouse}

	// The help screen takes the place of the document and of the inspector,
	// with the status bar left where it was: what file is open and whether it
	// has unsaved work in it are true of the session whatever is being read.
	//
	// It is drawn after the size has been checked rather than before, since a
	// terminal too small for the document is too small for this: a screen
	// showing part of what the keys do would leave a reader worse off than one
	// saying how much room pino needs.
	//
	// Nothing here reaches the layout. Help does not change what layoutFor is
	// given, so the room the document has does not move while help is up, which
	// is what keeps the cursor and the scroll exactly where they were left.
	if info.Mode == application.ModeHelp {
		help := m.theme.RenderHelp(m.width, m.height-statusBarRows)
		help = append(help, m.theme.RenderStatusBar(info, bar, m.width))

		return fullScreen(strings.Join(help, "\n"), m.mouse)
	}

	rows := make([]string, 0, m.height)

	window, start := visible(frame, l.BodyHeight)

	for i, line := range window {
		selected := start+i == frame.Cursor
		row := clip(m.theme.RenderLine(line, indentFor(info), selected), l.BodyWidth)

		// The band behind the selected row runs to the far side of the
		// document, not to the far side of the text. Where a row happens to
		// stop has nothing to do with which row is selected, so a highlight
		// stopping there reads as ragged rather than as a row being pointed
		// at. It stops at the document's own edge so that it cannot reach into
		// the inspector standing beside it.
		if selected {
			row += m.theme.RenderCursorFill(l.BodyWidth - ansi.StringWidth(row))
		}

		rows = append(rows, row)
	}

	// Blank rows hold the bar at the bottom when the document is shorter than
	// the screen.
	for len(rows) < l.BodyHeight {
		rows = append(rows, "")
	}

	rows = m.withInspector(rows, l)
	rows = append(rows, m.prompt(l)...)
	rows = append(rows, m.theme.RenderStatusBar(info, bar, m.width))

	return fullScreen(strings.Join(rows, "\n"), m.mouse)
}

// prompt is the band asking a question, in the rows the layout set aside for
// it.
//
// It is fitted to that height rather than drawn to whatever it turned out to
// need: a row too many would push the status bar off the screen, and a row too
// few would leave the document's last row showing through under it. The height
// and the band come from the same reading of the prompt, so the two agree
// except on a screen too short to hold what was asked for.
func (m Model) prompt(l layout) []string {
	if l.PromptHeight <= 0 {
		return nil
	}

	band := m.theme.RenderPrompt(m.app.Prompt(), m.editor.View(), m.width)

	return fitRows(band, m.width, l.PromptHeight)
}

// withInspector puts the inspector where the layout says it goes.
//
// Beside the document, each row of the pane is joined to the row of the
// document it sits next to, which is why the two are built as rows rather than
// as blocks: lipgloss would fill the ragged right of the document with plain
// spaces, and the band behind the selected row has to carry its own colour all
// the way to the rule. Under the document, there is nothing to join and the
// pane is simply stacked.
func (m Model) withInspector(rows []string, l layout) []string {
	switch l.Inspector {
	case placeSide:
		pane := m.theme.RenderInspectorPane(m.app.Inspector(), l.InspectorWidth, l.BodyHeight)
		rule := m.theme.RenderVerticalRule()

		// The selected row arrives already filled to this width, in the
		// cursor's own styling, so padding leaves it alone; the rest are
		// filled with plain space.
		for i := range rows {
			rows[i] = pad(rows[i], l.BodyWidth) + rule + pane[i]
		}

	case placeBelow:
		// The rule is counted in the height the layout set aside, so it comes
		// out of the pane's rows rather than out of the document's.
		if l.InspectorHeight <= 0 {
			break
		}

		rows = append(rows, m.theme.RenderHorizontalRule(m.width))
		rows = append(rows, m.theme.RenderInspectorStrip(
			m.app.Inspector(), m.width, l.InspectorHeight-ruleRows)...)

	case placeNone:
		// The JSON view has no pane. The status bar already names the pointer
		// and the type of the selection, which is what one would repeat.
	}

	return rows
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
//
// How tall the band is depends on what is being asked and on how far the box
// has grown, which is why the model works it out and hands over a number: the
// division of the screen stays arithmetic on sizes.
func (m Model) layout() layout {
	return layoutFor(m.width, m.height, m.app.ViewMode(),
		promptRows(m.app.Prompt(), m.editor.Rows()))
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

// visible is the part of the document that fits in height rows, along with the
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
func visible(frame application.Frame, height int) ([]documentview.Line, int) {
	start := min(max(frame.Scroll, 0), len(frame.Lines))
	end := min(start+max(height, 0), len(frame.Lines))

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
