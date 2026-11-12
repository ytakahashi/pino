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
	"fmt"
	"io"
	"io/fs"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application"
	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/infrastructure/filestore"
	"github.com/ytakahashi/pino/internal/infrastructure/jsonparser"
	"github.com/ytakahashi/pino/internal/infrastructure/textwrap"
	"github.com/ytakahashi/pino/internal/presentation"
)

// Exit codes. A misuse of the command line is told apart from a failure to do
// what was asked, so that a caller can distinguish "pino cannot be driven that
// way" from "this file could not be opened" without reading the message.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// Run opens the document named on the command line, or lists the directory
// named there to pick one from, and hands the terminal to it, returning the
// exit code the process should end with.
func Run(args []string, stdin Input, stdout, stderr io.Writer) int {
	parser := newOptionParser()

	// Parse errors take precedence over flags already seen, so -hx cannot turn
	// an invalid shorthand group into a successful help request.
	if err := parser.parse(args); err != nil {
		printf(stderr, "pino: %v\n", err)
		parser.usage(stderr)

		return exitUsage
	}

	options := parser.options

	if options.showHelp {
		parser.usage(stdout)

		return exitOK
	}

	if options.showVersion {
		printf(stdout, "%s\n", version())

		return exitOK
	}

	choice, err := documentFrom(options, stdin.Terminal)
	if err != nil {
		if !errors.Is(err, errUsageOnly) {
			printf(stderr, "pino: %v\n", err)
		}
		parser.usage(stderr)

		return exitUsage
	}

	cfg, err := configFrom(options)
	if err != nil {
		printf(stderr, "pino: %v\n", err)
		parser.usage(stderr)

		return exitUsage
	}

	var doc Document = FileDocument{Path: choice.path}
	if choice.stdin {
		data, err := io.ReadAll(stdin.Reader)
		if err != nil {
			printf(stderr, "pino: <stdin>: %v\n", err)

			return exitError
		}

		doc = StdinDocument{Data: data, Output: choice.output}
	} else if info, err := os.Stat(choice.path); err == nil && info.IsDir() {
		// A failed Stat keeps the file path: Open owns missing-file creation
		// and reports filesystem errors with the existing file diagnostics.
		doc = DirectoryDocument{Dir: choice.path}
	}

	// Open explicit files and stdin, or list a directory, before taking the
	// terminal. Picker selections are opened inside the running program so an
	// unreadable candidate can be reported while another is chosen.
	model, err := NewProgramModel(doc, cfg)
	if err != nil {
		if errors.Is(err, errNoJSONFiles) {
			printf(stderr, "pino: no .json files in %s\n", doc.label())

			return exitOK
		}
		reportOpenError(stderr, doc, err)

		return exitError
	}

	programOptions := []tea.ProgramOption{tea.WithOutput(stdout)}
	if choice.stdin {
		// Parse before opening the terminal so invalid input is reported even
		// without a controlling terminal. The consumed pipe cannot supply keys.
		in, out, err := tea.OpenTTY()
		if err != nil {
			var pathError *fs.PathError
			if errors.As(err, &pathError) {
				// Keep the OS cause without Bubble Tea's assembly details.
				err = pathError
			}

			printf(stderr, "pino: cannot open the terminal for keyboard input: %v\n", err)

			return exitError
		}
		// These handles only carry terminal input; closing them cannot change
		// a saved document or the outcome already reported by the program.
		defer func() { _ = in.Close() }()
		defer func() { _ = out.Close() }()

		programOptions = append(programOptions, tea.WithInput(in))
	}

	program := tea.NewProgram(model, programOptions...)

	if _, err := program.Run(); err != nil {
		printf(stderr, "pino: %v\n", err)

		return exitError
	}

	return exitOK
}

// ProgramConfig is every policy the command line passes into the assembled
// program. Application policies are nested rather than flattened so that a
// policy of the command line itself has somewhere to arrive without becoming
// a setting of the document session.
type ProgramConfig struct {
	Application application.Config
}

// NewProgramModel prepares the starting document or picker and its model.
//
// This is the assembly Run performs, kept apart from it because everything
// around it — the flags, the writers, the exit code — needs a process to mean
// anything, while the wiring itself does not. An end-to-end test drives the
// model this returns and so exercises the adapters Run would have chosen,
// rather than a second set of them written out beside it.
//
// The terminal is untouched here: a file is read and parsed or a directory
// is listed. Nothing is drawn until a program is given what comes back.
func NewProgramModel(doc Document, cfg ProgramConfig) (tea.Model, error) {
	app := application.New(application.Deps{
		Parser:   jsonparser.New(),
		Files:    filestore.New(),
		Wrapper:  textwrap.New(),
		JSONView: documentview.NewJSONRenderer(),
		TreeView: documentview.NewTreeRenderer(),
	}, cfg.Application)

	return doc.start(programParts{
		app:    app,
		lister: filestore.NewLister(),
		theme:  presentation.DefaultTheme(),
	})
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

// reportOpenError says in one line why the document could not be opened.
//
// Syntax errors use the document's label and any known position. Path errors
// already name the file; other errors need the label to identify the document.
func reportOpenError(w io.Writer, doc Document, err error) {
	var syntax *jsonparser.SyntaxError
	var pathError *fs.PathError

	switch {
	case !errors.As(err, &syntax):
		if errors.As(err, &pathError) {
			printf(w, "pino: %v\n", err)
		} else {
			printf(w, "pino: %s: %v\n", doc.label(), err)
		}

	case syntax.Line == 0:
		// The parser could not place the failure, which happens when the
		// library rewords its errors. The reason is still worth printing; a
		// guessed position would not be.
		printf(w, "pino: %s: %s\n", doc.label(), syntax.Msg)

	default:
		printf(w, "pino: %s:%d:%d: %s\n", doc.label(), syntax.Line, syntax.Column, syntax.Msg)
	}
}
