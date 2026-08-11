package e2e

import (
	"strings"
	"testing"
)

// A file is read from disk, parsed, laid out and moved through. Every piece of
// this is the real one, which is what the assertions turn on: the rows are
// indented the way the file on disk is, and the bar counts the lines of the
// document that was parsed rather than of one a test built.
func TestReadsAFileFromDisk(t *testing.T) {
	t.Parallel()

	tm := start(t, "localhost")

	// Down to the nested container, then into it.
	tm.Type("j")
	tm.Type("j")
	tm.Type("l")

	screen := finalScreen(t, tm)

	if got, want := screenRow(screen, 0), "{"; got != want {
		t.Errorf("row 0 = %q, want %q", got, want)
	}

	// Three levels down, drawn at the width the file itself is indented at.
	// Nothing but the file store and the parser could have said four.
	if got, want := screenRow(screen, 3), `            "ttl": 60`; got != want {
		t.Errorf("row 3 = %q, want %q", got, want)
	}

	// The bar names the node three keystrokes reached, in the file it was read
	// from, and describes the document rather than the part of it on screen.
	if got := statusRow(screen); !strings.Contains(got, "config.json  /server/cache/ttl  number") {
		t.Errorf("the bar reads %q, want it to name the file and the selection", got)
	}

	if got := statusRow(screen); !strings.Contains(got, "9 lines  indent:4") {
		t.Errorf("the bar reads %q, want the document as the file holds it", got)
	}
}
