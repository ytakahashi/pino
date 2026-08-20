package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/pflag"

	"github.com/ytakahashi/pino/internal/application"
)

// commandOptions is the command-line policy understood by Run. Keeping the
// parser-specific representation here prevents the CLI library from becoming
// part of the program assembly or application configuration.
type commandOptions struct {
	showHelp       bool
	showVersion    bool
	indent         int
	overrideIndent bool
	arguments      []string
}

type optionParser struct {
	flags   *pflag.FlagSet
	options commandOptions
}

func newOptionParser() *optionParser {
	parser := &optionParser{
		flags: pflag.NewFlagSet("pino", pflag.ContinueOnError),
	}

	parser.flags.SetOutput(io.Discard)
	parser.flags.Usage = func() {}

	// Register help explicitly instead of relying on pflag's implicit -h.
	// Otherwise -help stops at h and is accepted instead of being rejected as
	// an invalid group of short options.
	parser.flags.BoolVarP(&parser.options.showHelp, "help", "h", false, "print help information and exit")
	parser.flags.BoolVarP(&parser.options.showVersion, "version", "v", false, "print version information and exit")
	parser.flags.IntVarP(
		&parser.options.indent,
		"indent",
		"i",
		0,
		"indentation width, overriding the one detected in the file",
	)

	return parser
}

func (p *optionParser) parse(args []string) error {
	if err := p.flags.Parse(args); err != nil {
		return err
	}

	p.options.overrideIndent = p.flags.Changed("indent")
	p.options.arguments = p.flags.Args()

	return nil
}

func (p *optionParser) usage(w io.Writer) {
	printf(w, "usage: pino [options] <file> [options]\n\noptions:\n%s", p.flags.FlagUsages())
}

// configFrom is what the command line asked pino to do, as opposed to what it
// gave pino to do it with.
//
// Whether --indent was given is recorded separately from its value because a
// width of zero is a request in its own right: it asks for no indentation,
// while the flag being absent asks for the file's own.
//
// A width outside the range is refused rather than repaired. It is a misuse
// of the command line, and the exit code says so; guessing what was meant
// would make "-2" a way of writing "0".
func configFrom(options commandOptions) (ProgramConfig, error) {
	if !options.overrideIndent {
		return ProgramConfig{}, nil
	}

	if options.indent < 0 || options.indent > maxIndent {
		return ProgramConfig{}, fmt.Errorf(
			"indentation width %d is not between 0 and %d", options.indent, maxIndent)
	}

	// Spaces, which is the only width a number can mean. A file indented with
	// tabs keeps them by not being overridden at all.
	appCfg := application.Config{
		IndentOverride: strings.Repeat(" ", options.indent),
		OverrideIndent: true,
	}

	return ProgramConfig{Application: appCfg}, nil
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
