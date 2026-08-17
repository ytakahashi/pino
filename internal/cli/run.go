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
	"strings"

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

	indent := fs.Int("indent", 0, "indentation width, overriding the one detected in the file")
	noMouse := fs.Bool("no-mouse", false, "disable mouse reporting to allow terminal text selection")

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

	cfg, err := configFrom(fs, *indent, *noMouse)
	if err != nil {
		printf(stderr, "pino: %v\n", err)
		usage(stderr, fs)

		return exitUsage
	}

	path := fs.Arg(0)

	// The document is opened before the terminal is taken over: pino is a
	// structural editor and has no way to repair a broken file, so the reason
	// it cannot be opened is worth more on the terminal the user launched pino
	// from than on a screen that would have nothing to offer them.
	model, err := NewProgramModel(path, cfg)
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

// ProgramConfig is every policy the command line passes into the assembled
// program. Application policies remain nested so terminal concerns cannot
// leak into the document session.
type ProgramConfig struct {
	Application  application.Config
	DisableMouse bool
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
func NewProgramModel(path string, cfg ProgramConfig) (tea.Model, error) {
	app := application.New(application.Deps{
		Parser:   jsonparser.New(),
		Files:    filestore.New(),
		JSONView: documentview.NewJSONRenderer(),
		TreeView: documentview.NewTreeRenderer(),
	}, cfg.Application)

	if err := app.Open(path); err != nil {
		return nil, err
	}

	return presentation.NewModel(app, presentation.DefaultTheme(), presentation.ModelConfig{
		DisableMouse: cfg.DisableMouse,
	}), nil
}

// configFrom is what the command line asked pino to do, as opposed to what it
// gave pino to do it with.
//
// Whether --indent was given is asked of the set rather than read off the
// value, because a width of zero is a request in its own right: it asks for no
// indentation, while the flag being absent asks for the file's own. A default
// value cannot say which of the two happened.
//
// A width outside the range is refused rather than repaired. It is a misuse
// of the command line, and the exit code says so; guessing what was meant
// would make "-2" a way of writing "0".
//
// disableMouse is carried straight through: there is nothing to validate in a
// bool, and no default to tell apart from a choice. It is a parameter here
// rather than something Run puts on afterwards so that every flag lands in the
// config in one place, and so that a test can hold the whole of that reading
// to the flags it was given. Its being dropped from the config returned
// alongside an error is of no consequence, since a caller that got an error
// has a usage message to print and nothing to configure.
func configFrom(fs *flag.FlagSet, indent int, disableMouse bool) (ProgramConfig, error) {
	var appCfg application.Config

	fs.Visit(func(f *flag.Flag) {
		if f.Name == "indent" {
			appCfg.OverrideIndent = true
		}
	})

	if !appCfg.OverrideIndent {
		return ProgramConfig{DisableMouse: disableMouse}, nil
	}

	if indent < 0 || indent > maxIndent {
		return ProgramConfig{}, fmt.Errorf(
			"indentation width %d is not between 0 and %d", indent, maxIndent)
	}

	// Spaces, which is the only width a number can mean. A file indented with
	// tabs keeps them by not being overridden at all.
	appCfg.IndentOverride = strings.Repeat(" ", indent)

	return ProgramConfig{Application: appCfg, DisableMouse: disableMouse}, nil
}

// maxIndent is the widest level pino will write.
//
// JSON forbids nothing here, and the limit is not about JSON. The number
// arrives from a command line, and every space of it is allocated and then
// drawn on every row of every level: without a bound, a mistyped width is a
// request to build a string of whatever the flag parser accepted, which for
// the largest of them is not a string that can exist.
//
// Documents are written with two spaces or four. Eight is the width a tab has
// traditionally been drawn at, and twice what anything here is likely to ask
// for, so it is past the useful range rather than in the middle of it: at the
// narrowest terminal pino draws in, a document four levels deep at this width
// has nothing left of the row.
const maxIndent = 8

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
