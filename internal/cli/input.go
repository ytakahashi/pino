package cli

import (
	"io"
	"os"

	"github.com/charmbracelet/x/term"
)

// Input describes standard input independently of the process running Run.
type Input struct {
	Reader   io.Reader
	Terminal bool
}

// StandardInput describes f, which is os.Stdin in the running program.
func StandardInput(f *os.File) Input {
	return Input{Reader: f, Terminal: term.IsTerminal(f.Fd())}
}
