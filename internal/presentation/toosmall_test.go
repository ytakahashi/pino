package presentation

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// A terminal too small to draw in. Pino says so rather than drawing something
// misleading, and comes back when there is room again.

// Below the size pino draws in, the screen says why rather than showing part
// of a document that cannot be arranged in the room left.
func TestViewSaysWhenTheTerminalIsTooSmall(t *testing.T) {
	got := rows(t, sized(t, openTestApp(t), 34, 6))

	if len(got) != 6 {
		t.Fatalf("View() drew %d rows, want 6", len(got))
	}

	want := []string{"terminal too small", "needs 60x12, has 34x6", "", "", "", ""}

	for i, w := range want {
		if got[i] != w {
			t.Errorf("row %d = %q, want %q", i, got[i], w)
		}
	}
}

// The warning is not a mode. The session is running behind it, so the keys go
// on meaning what they meant — and a screen that could not be left would be a
// worse answer than one that cannot be read.
func TestViewTooSmallStillTakesKeys(t *testing.T) {
	m := sized(t, openApp(t, longDocument(t)), 40, 6)

	if !m.layout().TooSmall {
		t.Fatal("the terminal is not too small, so this is testing nothing")
	}

	m = press(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

	if got := m.app.Status().Pointer; got != "/k0" {
		t.Errorf("j selected %q behind the warning, want /k0", got)
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q produced no command behind the warning, want a quit")
	}

	if msg := cmd(); !isQuit(msg) {
		t.Errorf("q produced %T behind the warning, want tea.QuitMsg", msg)
	}
}

// Widening the terminal brings the document back as it was left: the warning
// covered the session rather than replacing it.
func TestViewComesBackWhenTheTerminalGrows(t *testing.T) {
	m := sized(t, openApp(t, nestedDocument(t)), 80, 24)

	// Somewhere into the document, with a container folded away.
	m = press(t, m,
		tea.KeyPressMsg{Code: 'j', Text: "j"},
		tea.KeyPressMsg{Code: 'j', Text: "j"},
		tea.KeyPressMsg{Code: 'h', Text: "h"},
	)

	before := m.app.Status()
	if before.Pointer != "/server/cache" {
		t.Fatalf("the cursor is at %q, want /server/cache", before.Pointer)
	}

	drawn := rows(t, m)

	// Too small, and the document is gone from the screen.
	m = sizedFrom(t, m, 34, 6)

	if got := rows(t, m)[0]; got != "terminal too small" {
		t.Fatalf("the screen reads %q, want the warning", got)
	}

	// And back, unchanged.
	m = sizedFrom(t, m, 80, 24)

	if got := m.app.Status().Pointer; got != before.Pointer {
		t.Errorf("the cursor is at %q after the terminal grew, want %q", got, before.Pointer)
	}

	if got := rows(t, m); !slices.Equal(got, drawn) {
		t.Errorf("the screen came back as\n%v\nwant\n%v", got, drawn)
	}
}
