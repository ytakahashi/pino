package cli

import "github.com/ytakahashi/pino/internal/application"

// Document is what a program opens. Its unexported methods limit the choices
// to the sources supported by the program assembly.
type Document interface {
	open(*application.App) error
	label() string
}

// FileDocument opens a file, or starts a new document at a missing path.
type FileDocument struct{ Path string }

func (d FileDocument) open(app *application.App) error { return app.Open(d.Path) }
func (d FileDocument) label() string                   { return d.Path }

// StdinDocument opens bytes read from standard input. Output is an optional
// destination, which becomes the document's path only after a committed save.
type StdinDocument struct {
	Data   []byte
	Output string
}

func (d StdinDocument) open(app *application.App) error { return app.OpenStdin(d.Data, d.Output) }
func (StdinDocument) label() string                     { return "<stdin>" }
