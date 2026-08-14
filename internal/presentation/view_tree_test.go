package presentation

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
)

// The other view: the tree, and the inspector that is drawn beside or under it
// depending on the room there is.

// Tab redraws the same document the other way. This is the whole of what the
// key does on screen: the same node stays selected, and the rows around it
// change shape.
func TestViewSwitchesToTheTree(t *testing.T) {
	m := sized(t, openApp(t, nestedDocument(t)), 60, 20)

	// Down to a node inside the document, so that keeping the selection is
	// something more than staying on the root.
	m = press(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"}, tea.KeyPressMsg{Code: 'j', Text: "j"})

	before := m.app.Status()
	if before.Pointer != "/server/cache" {
		t.Fatalf("the cursor is at %q, want /server/cache", before.Pointer)
	}

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") != "{" {
		t.Fatalf("the JSON view begins with %q, want a brace", got)
	}

	m = press(t, m, tabKey)

	after := m.app.Status()

	if after.ViewMode != application.ViewTree {
		t.Errorf("the bar names the %v view after Tab, want %v", after.ViewMode, application.ViewTree)
	}

	if after.Pointer != before.Pointer {
		t.Errorf("the cursor moved to %q, want %q", after.Pointer, before.Pointer)
	}

	// The root of the tree: a marker, the name pino gives the root, and the
	// count of what it holds.
	if got := rows(t, m)[0]; strings.TrimRight(got, " ") != "▼ / {2}" {
		t.Errorf("the tree view begins with %q, want %q", got, "▼ / {2}")
	}

	// And back, to the document as it is written.
	m = press(t, m, tabKey)

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") != "{" {
		t.Errorf("the JSON view begins with %q after switching back, want a brace", got)
	}
}

// The tree is drawn two columns per level however the file is laid out, while
// the bar goes on reporting what saving will do.
func TestViewDrawsTheTreeWithItsOwnIndent(t *testing.T) {
	m := sized(t, openIndented(t, nestedDocument(t), "\t"), 60, 20)

	// The JSON view follows the file: one tab per level.
	if got := rows(t, m)[1]; !strings.HasPrefix(got, "\t\"server\"") {
		t.Errorf("the JSON view draws %q, want a tab in front of the key", got)
	}

	m = press(t, m, tabKey)

	drawn := rows(t, m)

	// The tree does not: two spaces per level, and none of the file's tabs.
	if got, want := drawn[1], "  ▼ server {2}"; got != want {
		t.Errorf("the tree view draws %q, want %q", got, want)
	}

	if got := drawn[2]; !strings.HasPrefix(got, "    ▼ cache {1}") {
		t.Errorf("a row two levels deep is %q, want four columns of indentation", got)
	}

	// The bar still says what the document uses, because that is what saving
	// will write rather than what the screen is doing.
	bar := drawn[len(drawn)-1]
	if !strings.Contains(bar, "indent:tab") {
		t.Errorf("the bar reads %q, want it to still report the document's indent", bar)
	}
}

// On a terminal wide enough for the inspector to stand beside the tree, each
// row of the screen is a row of the document, the rule, and a row of the pane.
// The rule stands in the same column throughout, whatever each row happens to
// hold.
func TestViewJoinsTheInspectorBesideTheTree(t *testing.T) {
	const width = 120

	m := press(t, sized(t, openApp(t, nestedDocument(t)), width, 20), tabKey)

	l := m.layout()
	if l.Inspector != placeSide {
		t.Fatalf("the inspector is placed %v on a %d column terminal, want beside", l.Inspector, width)
	}

	drawn := rows(t, m)

	// The status bar is the one row that is not divided.
	for i, row := range drawn[:len(drawn)-1] {
		if got := lipgloss.Width(row); got != width {
			t.Errorf("row %d is %d wide, want the full %d: %q", i, got, width, row)
		}

		if got := []rune(row)[l.BodyWidth]; got != '│' {
			t.Errorf("row %d has %q where the rule belongs, want the rule: %q", i, got, row)
		}
	}

	// The document is on the left of it and the pane on the right.
	if got, want := strings.TrimRight(bodyOf(drawn[0], l), " "), "▼ / {2}"; got != want {
		t.Errorf("the first row of the document is %q, want %q", got, want)
	}

	if got, want := strings.TrimSpace(paneOf(drawn[0], l)), "Path"; got != want {
		t.Errorf("the pane begins with %q, want %q", got, want)
	}
}

// The band behind the selected row stops at the rule. Reaching past it would
// paint the pane as though the selection were in it.
func TestViewKeepsTheCursorBandOutOfTheInspector(t *testing.T) {
	m := press(t, sized(t, openApp(t, nestedDocument(t)), 120, 20), tabKey)

	l := m.layout()
	styled := strings.Split(m.View().Content, "\n")[selectedRow(t, m)]

	// The document's side of the row is filled with the band all the way to
	// the rule, so that the mark is not ragged.
	if got := lipgloss.Width(bodyOf(ansi.Strip(styled), l)); got != l.BodyWidth {
		t.Errorf("the selected row is %d wide before the rule, want the body's %d", got, l.BodyWidth)
	}

	marker := cursorBackground(t, m.theme)

	rule := strings.Index(styled, "│")
	if rule < 0 {
		t.Fatalf("the selected row holds no rule: %q", styled)
	}

	if rest := styled[rule:]; strings.Contains(rest, marker) {
		t.Errorf("the cursor's background reaches into the pane: %q", rest)
	}
}

// Under the tree, the pane is stacked below the document with a rule between
// them, and the rows add up to the height of the terminal.
func TestViewStacksTheInspectorUnderTheTree(t *testing.T) {
	const width, height = 80, 20

	m := press(t, sized(t, openApp(t, nestedDocument(t)), width, height), tabKey)

	l := m.layout()
	if l.Inspector != placeBelow {
		t.Fatalf("the inspector is placed %v on a %d column terminal, want below", l.Inspector, width)
	}

	drawn := rows(t, m)

	if len(drawn) != height {
		t.Fatalf("View() drew %d rows, want %d", len(drawn), height)
	}

	// The rule sits between the last row of the document and the first of the
	// pane, and runs the whole way across.
	if got := drawn[l.BodyHeight]; got != strings.Repeat("─", width) {
		t.Errorf("the row below the document is %q, want a rule", got)
	}

	// One field to a row, the values lined up under one another.
	want := []string{" Path      /", " Type      object", " Children  2", " Keys      t a", ""}

	for i, w := range want {
		if got := strings.TrimRight(drawn[l.BodyHeight+1+i], " "); got != w {
			t.Errorf("row %d of the pane is %q, want %q", i, got, w)
		}
	}
}
