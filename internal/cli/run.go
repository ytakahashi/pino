// Package cli turns a command line into a pino session.
//
// This is where the program is assembled: the adapters are built here and
// handed to the application as ports, which makes it the only package allowed
// to name a concrete one. Everything it does is expressed as writes to the
// given writers and an exit code, so a run can be examined without starting a
// process.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application"
	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/infrastructure/filestore"
	"github.com/ytakahashi/pino/internal/infrastructure/jsonparser"
	"github.com/ytakahashi/pino/internal/presentation"
)

// Exit codes. A misuse of the command line is told apart from a failure to do
// what was asked, following the convention the flag package itself follows, so
// that a caller can distinguish "pino cannot be driven that way" from "this
// file could not be opened" without reading the message.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// Run opens the document named on the command line and hands the terminal to
// it, returning the exit code the process should end with.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("pino", flag.ContinueOnError)

	// The flag package writes both its errors and its usage to one writer and
	// prints the usage itself when help is asked for. Silencing it leaves this
	// function to decide where each goes, so that requested help reaches stdout
	// while a misuse reaches stderr.
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	showVersion := fs.Bool("version", false, "print version information and exit")

	// Accepted and not read. The width only takes effect when a document is
	// written back, which pino cannot do yet; taking the flag now keeps the
	// command line it will be part of settled.
	fs.Int("indent", 0, "indentation width, overriding the one detected in the file")

	switch err := fs.Parse(args); {
	case errors.Is(err, flag.ErrHelp):
		usage(stdout, fs)

		return exitOK

	case err != nil:
		printf(stderr, "pino: %v\n", err)
		usage(stderr, fs)

		return exitUsage
	}

	if *showVersion {
		printf(stdout, "%s\n", version())

		return exitOK
	}

	// Exactly one document. Reading from standard input and holding several
	// files at once are both foreseen, and both need more than an argument to
	// arrive, so anything else is a misuse rather than a shortcut.
	if fs.NArg() != 1 {
		usage(stderr, fs)

		return exitUsage
	}

	path := fs.Arg(0)

	// The document is opened before the terminal is taken over: pino is a
	// structural editor and has no way to repair a broken file, so the reason
	// it cannot be opened is worth more on the terminal the user launched pino
	// from than on a screen that would have nothing to offer them.
	model, err := NewProgramModel(path)
	if err != nil {
		reportOpenError(stderr, path, err)

		return exitError
	}

	program := tea.NewProgram(
		model,
		// Output is the writer this function was given rather than os.Stdout,
		// so that everything the run puts on the terminal goes through the
		// same place.
		tea.WithOutput(stdout),
	)

	if _, err := program.Run(); err != nil {
		printf(stderr, "pino: %v\n", err)

		return exitError
	}

	return exitOK
}

// NewProgramModel opens a document and answers the model that draws it.
//
// This is the assembly Run performs, kept apart from it because everything
// around it — the flags, the writers, the exit code — needs a process to mean
// anything, while the wiring itself does not. An end-to-end test drives the
// model this returns and so exercises the adapters Run would have chosen,
// rather than a second set of them written out beside it.
//
// The terminal is untouched here: the document is read, parsed and laid out,
// and nothing is drawn until a program is given what comes back.
func NewProgramModel(path string) (tea.Model, error) {
	app := application.New(application.Deps{
		Parser:   jsonparser.New(),
		Files:    filestore.New(),
		JSONView: documentview.NewJSONRenderer(),
		TreeView: documentview.NewTreeRenderer(),
	})

	if err := app.Open(path); err != nil {
		return nil, err
	}

	return presentation.NewModel(app, presentation.DefaultTheme()), nil
}

// printf puts a message on the terminal.
//
// A failed write is dropped, here and nowhere else in pino. These writers are
// the process's own output, and a program that cannot say why it is stopping
// has no second way to say it either; the exit code carries the outcome
// regardless.
func printf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// usage describes how pino is invoked.
func usage(w io.Writer, fs *flag.FlagSet) {
	printf(w, "usage: pino [options] <file>\n\noptions:\n")

	// PrintDefaults writes to the set's own output, which Run silenced in
	// order to choose the destination per message.
	fs.SetOutput(w)
	fs.PrintDefaults()
}

// reportOpenError says in one line why the document could not be opened.
//
// A position is added only for the errors that carry one. Everything the file
// store reports is a *fs.PathError, which names the path itself, so repeating
// it here would print the file twice.
func reportOpenError(w io.Writer, path string, err error) {
	var syntax *jsonparser.SyntaxError

	switch {
	case !errors.As(err, &syntax):
		printf(w, "pino: %v\n", err)

	case syntax.Line == 0:
		// The parser could not place the failure, which happens when the
		// library rewords its errors. The reason is still worth printing; a
		// guessed position would not be.
		printf(w, "pino: %s: %s\n", path, syntax.Msg)

	default:
		printf(w, "pino: %s:%d:%d: %s\n", path, syntax.Line, syntax.Column, syntax.Msg)
	}
}
