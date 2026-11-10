package presentation

import (
	"errors"
	"fmt"
	"io/fs"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application"
	"github.com/ytakahashi/pino/internal/application/picker"
)

// RootModel connects a file picker to the document session. Exactly one child
// is active: a successful open discards the picker, with no route back to it.
type RootModel struct {
	app    *application.App
	theme  Theme
	picker *pickerModel
	editor *Model
	width  int
	height int
	// The root owns mouse reporting while picking; the editor takes it over
	// when the chosen document is opened.
	mouse bool
}

// NewRootModel starts with an already listed picker and an unopened session.
// Listing is the caller's responsibility so startup failures can be reported
// before the program takes over the terminal.
func NewRootModel(app *application.App, p *picker.Picker, theme Theme) RootModel {
	return RootModel{app: app, theme: theme, picker: &pickerModel{picker: p, theme: theme}, mouse: true}
}

func (m RootModel) Init() tea.Cmd { return nil }

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
	}
	if m.editor != nil {
		return m.forward(msg)
	}

	next, effects, terminal := m.picker.update(msg, m.mouse)
	m.picker = &next
	switch terminal {
	case TerminalToggleMouse:
		m.mouse = !m.mouse
	case TerminalNone:
	}

	for _, effect := range effects {
		switch e := effect.(type) {
		case picker.EffectOpen:
			if err := m.app.OpenExisting(e.Path); err != nil {
				// File-store errors already name their path; parser errors do
				// not. Preserve both the cause and a readable filename.
				var pathErr *fs.PathError
				if !errors.As(err, &pathErr) {
					err = fmt.Errorf("%s: %w", e.Path, err)
				}
				m.picker.picker.Failed(err)
				m.picker.syncSize()

				continue
			}

			// The program started the root, not the editor, so its Init runs
			// here. A document opened from the picker must start the same way
			// as one named on the command line, whatever Init comes to do.
			editor := newModel(m.app, m.theme, m.mouse)
			initCmd := editor.Init()
			m.editor = &editor
			m.picker = nil

			// Size the editor before its first frame, since no new terminal
			// resize is guaranteed to arrive after switching children.
			next, sizeCmd := m.forward(tea.WindowSizeMsg{Width: m.width, Height: m.height})

			return next, tea.Batch(initCmd, sizeCmd)

		case picker.EffectQuit:
			return m, tea.Quit

		case picker.EffectBeginInput:
			// The picker model has already created its input widget.
		}
	}

	return m, nil
}

// forward hands msg to the editor. Model.Update returns its own type behind
// tea.Model; the assertion keeps the editor concrete for the root's lifetime.
func (m RootModel) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.editor.Update(msg)
	editor := next.(Model)
	m.editor = &editor

	return m, cmd
}

func (m RootModel) View() tea.View {
	if m.editor != nil {
		return m.editor.View()
	}

	return m.picker.view(m.mouse)
}
