package cli

import (
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application"
	"github.com/ytakahashi/pino/internal/application/picker"
	"github.com/ytakahashi/pino/internal/presentation"
)

// Document is what a program starts with. Its unexported methods limit the
// choices to the sources supported by the program assembly.
type Document interface {
	start(programParts) (tea.Model, error)
	label() string
}

// programParts holds the dependencies assembled for a starting document.
type programParts struct {
	app    *application.App
	lister picker.DirLister
	theme  presentation.Theme
}

// FileDocument opens a file, or starts a new document at a missing path.
type FileDocument struct{ Path string }

func (d FileDocument) start(p programParts) (tea.Model, error) {
	if err := p.app.Open(d.Path); err != nil {
		return nil, err
	}
	return presentation.NewModel(p.app, p.theme), nil
}
func (d FileDocument) label() string { return d.Path }

// StdinDocument opens bytes read from standard input. Output is an optional
// destination, which becomes the document's path only after a committed save.
type StdinDocument struct {
	Data   []byte
	Output string
}

func (d StdinDocument) start(p programParts) (tea.Model, error) {
	if err := p.app.OpenStdin(d.Data, d.Output); err != nil {
		return nil, err
	}
	return presentation.NewModel(p.app, p.theme), nil
}
func (StdinDocument) label() string { return "<stdin>" }

// DirectoryDocument starts a picker for the JSON files directly in Dir.
type DirectoryDocument struct{ Dir string }

// An empty initial list ends successfully before the terminal is taken over.
var errNoJSONFiles = errors.New("no .json files")

func (d DirectoryDocument) start(p programParts) (tea.Model, error) {
	list, err := picker.New(p.lister, d.Dir)
	if err != nil {
		return nil, err
	}
	if list.Empty() {
		return nil, errNoJSONFiles
	}
	return presentation.NewRootModel(p.app, list, p.theme), nil
}
func (d DirectoryDocument) label() string { return d.Dir }
