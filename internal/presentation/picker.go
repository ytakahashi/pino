package presentation

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application/picker"
)

// pickerModel owns only the terminal details of choosing a file. It returns
// effects to the root, the only model that knows how to open a document.
type pickerModel struct {
	picker  *picker.Picker
	theme   Theme
	width   int
	height  int
	pending Pending
	editor  editor
}

func (m pickerModel) update(msg tea.Msg, mouse bool) (pickerModel, []picker.Effect, TerminalAction) {
	var act picker.Action
	terminal := TerminalNone

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.editor = m.editor.SetWidth(m.width - 1)
		m.syncSize()

		return m, nil, terminal

	case tea.KeyPressMsg:
		act, terminal, m.pending = resolvePicker(msg, m.picker.Mode(), m.pending)
		if m.picker.Mode() == picker.ModeFilter && act == nil {
			m, act = m.typed(msg)
		}

	case tea.PasteMsg:
		if m.picker.Mode() != picker.ModeFilter {
			return m, nil, terminal
		}

		m.editor = m.editor.Update(msg)
		act = picker.ActionFilterChange{Text: m.editor.Value()}

	case tea.MouseWheelMsg:
		if !mouse || m.picker.Mode() != picker.ModeNormal {
			return m, nil, terminal
		}

		rows, ok := wheelDistance(msg)
		if !ok {
			return m, nil, terminal
		}

		act = picker.ActionScrollBy{Rows: rows}

	default:
		return m, nil, terminal
	}

	// Even an unbound key or a prefix dismisses the previous error. Do(nil)
	// clears it without inventing a picker action for terminal-only input.
	effects := m.picker.Do(act)
	for _, effect := range effects {
		if e, ok := effect.(picker.EffectBeginInput); ok {
			box, held := newLineEditor(m.theme, e.Text, m.width-1)
			if held {
				m.editor = box
			} else {
				m.picker.Do(picker.ActionFilterCancel{})
			}
		}
	}

	if m.picker.Mode() != picker.ModeFilter {
		m.editor = editor{}
	}
	m.syncSize()

	return m, effects, terminal
}

// typed hands a key press to the filter box, after taking the ones that end
// typing. Enter and Esc are taken here together, as the editor's typed does
// for a prompt, because the widget would otherwise consume them as text.
func (m pickerModel) typed(msg tea.KeyPressMsg) (pickerModel, picker.Action) {
	switch msg.String() {
	case "enter":
		return m, picker.ActionFilterSubmit{Text: m.editor.Value()}

	case "esc":
		return m, picker.ActionFilterCancel{}
	}

	m.editor = m.editor.Update(msg)

	return m, picker.ActionFilterChange{Text: m.editor.Value()}
}

// listHeight is the rows left for names in a frame f of the picker's state.
func (m pickerModel) listHeight(f picker.Frame) int {
	rows := m.height - statusBarRows
	if m.picker.Mode() == picker.ModeFilter {
		rows--
	}
	if f.Err != nil {
		rows--
	}

	return max(rows, 0)
}

func (m pickerModel) syncSize() { m.picker.SetSize(m.listHeight(m.picker.Frame())) }

func (m pickerModel) view(mouse bool) tea.View {
	if m.width <= 0 || m.height <= 0 {
		return fullScreen("", mouse)
	}
	if m.width < minWidth || m.height < minHeight {
		return fullScreen(m.theme.RenderTooSmall(m.width, m.height), mouse)
	}

	f := m.picker.Frame()
	var rows []string
	if m.picker.Mode() == picker.ModeHelp {
		rows = m.theme.renderHelp(pickerHelpTitle(m.width), pickerHelpLines(), m.width, m.height-statusBarRows)
	} else {
		height := m.listHeight(f)
		for i := f.Scroll; i < min(len(f.Names), f.Scroll+height); i++ {
			row := ansi.Truncate(escaped(f.Names[i]), m.width, "…")
			if i == f.Cursor {
				row = m.theme.Cursor.Render(pad(row, m.width))
			}
			rows = append(rows, row)
		}

		if len(f.Names) == 0 {
			message := "no matches"
			if f.Total == 0 {
				message = "no .json files"
			}
			rows = append(rows, m.theme.PromptHint.Render(message))
		}
		rows = fitRows(rows, m.width, height)

		if f.Err != nil {
			rows = append(rows, m.theme.PromptError.Render(ansi.Truncate(escaped(f.Err.Error()), m.width, "…")))
		}
		if m.picker.Mode() == picker.ModeFilter {
			rows = append(rows, clip(m.theme.Prompt.Render("/")+m.editor.View()[0], m.width))
		}
	}

	left := escaped(f.Dir)
	if f.Filter != "" {
		left += "  /" + escaped(f.Filter)
	}
	right := fmt.Sprintf("%d/%d", len(f.Names), f.Total)
	if !mouse {
		right += "  select:on"
	}
	if m.pending != PendingNone {
		right += "  " + m.pending.String()
	}
	rows = append(rows, m.theme.StatusBar.Render(spread(left, right, m.width)))

	return fullScreen(strings.Join(rows, "\n"), mouse)
}
