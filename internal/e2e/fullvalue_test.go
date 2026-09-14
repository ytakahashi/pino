package e2e

import (
	"strings"
	"testing"
)

// S writes the selected value out beneath its row, the block goes with the
// cursor, and S again takes it away. The rows are cut to the width the
// terminal reports, which is the part only a whole program puts together.
func TestTheProgramWritesTheSelectedValueOutInFull(t *testing.T) {
	t.Parallel()

	tm, waiter, _ := startAt(t, writeLongValues(t), "8080")

	tm.Type("j")
	tm.Type("S")

	screen := waiter.wait(t, func(screen []string) bool {
		return strings.HasPrefix(screenRow(screen, 2), `"note note`)
	})

	if got := statusRow(screen); !strings.Contains(got, "value:full") {
		t.Errorf("the bar reads %q, want it to name values shown in full", got)
	}

	if got := len([]rune(screenRow(screen, 2))); got != 80 {
		t.Errorf("the first row of the block is %d columns, want the 80 the terminal has", got)
	}

	// The next member moves up to sit directly under the note, and its value is
	// written out beneath it instead.
	tm.Type("j")

	screen = waiter.wait(t, func(screen []string) bool {
		return strings.HasPrefix(screenRow(screen, 3), `"other other`)
	})

	for i, row := range screen {
		if strings.HasPrefix(row, `"note note`) {
			t.Errorf("row %d = %q, want the note's block gone once the cursor left it", i, row)
		}
	}

	tm.Type("S")

	screen = finalScreen(t, tm)

	if got := screenRow(screen, 3); !strings.Contains(got, `"port": 8080`) {
		t.Errorf("row 3 = %q, want the port directly beneath the other member", got)
	}

	if got := statusRow(screen); strings.Contains(got, "value:full") {
		t.Errorf("the bar reads %q, want values no longer shown in full", got)
	}
}
