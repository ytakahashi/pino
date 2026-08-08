// Command pino is an interactive terminal editor for JSON files.
package main

import (
	"os"

	"github.com/ytakahashi/pino/internal/cli"
)

// main hands the process over and turns what comes back into an exit code.
// Everything else lives in cli, where it can be run without a process.
func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
