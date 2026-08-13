package presentation

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ytakahashi/pino/internal/application"
)

// What View draws of a document: how much of it reaches the screen, and how
// the row the cursor is on is told apart from the rest.

// Bubble Tea draws before it has said how big the terminal is, and a layout
// cannot be worked out from a size that is not known yet. The empty frame is
// replaced as soon as the size arrives.
func TestViewIsEmptyBeforeTheSizeIsKnown(t *testing.T) {
	m := NewModel(openTestApp(t), DefaultTheme())

	if got := m.View().Content; got != "" {
		t.Errorf("View() = %q, want %q", got, "")
	}
}

func TestViewFillsTheScreen(t *testing.T) {
	m := sized(t, openTestApp(t), 60, 10)

	got := rows(t, m)
	if len(got) != 10 {
		t.Fatalf("View() drew %d rows, want 10", len(got))
	}

	want := []string{
		"{",
		`  "host": "localhost",`,
		`  "port": 8080`,
		"}",
		"", "", "", "", "",
	}

	for i, w := range want {
		if strings.TrimRight(got[i], " ") != w {
			t.Errorf("row %d = %q, want %q", i, got[i], w)
		}
	}

	// The bar is anchored to the last row, whatever the document is worth,
	// with where the selection is at one end and what the document is at the
	// other.
	bar := got[9]

	if !strings.HasPrefix(bar, " NORMAL  JSON  config.json  /  object") {
		t.Errorf("the bar begins %q, want the session and the selection", bar)
	}

	if !strings.HasSuffix(bar, "4 lines  indent:2 ") {
		t.Errorf("the bar ends %q, want the state of the document", bar)
	}
}

// A document taller than the screen is shown from the top and cut off. It is
// the whole document that is counted in the bar, not the part on screen.
func TestViewCutsTheDocumentToTheBodyHeight(t *testing.T) {
	m := sized(t, openApp(t, longDocument(t)), 60, 10)

	got := rows(t, m)
	if len(got) != 10 {
		t.Fatalf("View() drew %d rows, want 10", len(got))
	}

	if want := "{"; strings.TrimRight(got[0], " ") != want {
		t.Errorf("row 0 = %q, want %q", got[0], want)
	}

	if want := `  "k0": 0,`; strings.TrimRight(got[1], " ") != want {
		t.Errorf("row 1 = %q, want %q", got[1], want)
	}

	if !strings.Contains(got[9], "32 lines") {
		t.Errorf("status bar = %q, want the whole document counted", got[9])
	}
}

// Every row is at most as wide as the terminal, so that a long line takes one
// row rather than wrapping onto the next and displacing everything below it.
func TestViewClipsRowsToTheWidth(t *testing.T) {
	const width = minWidth

	m := sized(t, openApp(t, wideDocument(t)), width, 10)

	for i, row := range rows(t, m) {
		if w := lipgloss.Width(row); w > width {
			t.Errorf("row %d is %d wide, want at most %d: %q", i, w, width, row)
		}
	}
}

// Nothing is open until a document has been read, and drawing has to survive
// that: the program is on the screen before the first file arrives.
func TestViewDrawsAnEmptyScreenWithoutADocument(t *testing.T) {
	app := application.New(application.Deps{
		Parser:   fakeParser{},
		Files:    fakeFileStore{},
		JSONView: application.NewJSONRenderer(),
		TreeView: application.NewTreeRenderer(),
	})

	got := rows(t, sized(t, app, 60, 10))
	if len(got) != 10 {
		t.Fatalf("View() drew %d rows, want 10", len(got))
	}

	for i, row := range got[:9] {
		if strings.TrimRight(row, " ") != "" {
			t.Errorf("row %d = %q, want it blank", i, row)
		}
	}

	// Nothing is selected, so the bar says only what is true of the session.
	bar := got[9]

	if !strings.HasPrefix(bar, " NORMAL  JSON") {
		t.Errorf("the bar begins %q, want the mode and the view", bar)
	}

	if !strings.HasSuffix(bar, "0 lines  indent:2 ") {
		t.Errorf("the bar ends %q, want the state of the document", bar)
	}
}

// However absurd the terminal, pino draws exactly the screen it was given and
// not a column more. The warning is what fills it below the minimum, and it
// has to fit there too.
func TestViewSurvivesATinyTerminal(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 1, height: 1},
		{width: 1, height: 24},
		{width: 80, height: 1},
		{width: 80, height: 2},
		{width: 3, height: 3},
	}

	for _, size := range sizes {
		t.Run(strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
			got := rows(t, sized(t, openTestApp(t), size.width, size.height))

			if len(got) != size.height {
				t.Errorf("View() drew %d rows, want %d", len(got), size.height)
			}

			for i, row := range got {
				if w := lipgloss.Width(row); w > size.width {
					t.Errorf("row %d is %d wide, want at most %d", i, w, size.width)
				}
			}
		})
	}
}

// The row the cursor is on is the one marked, and moving moves the mark.
func TestViewMarksTheCursorRow(t *testing.T) {
	m := sized(t, openTestApp(t), 60, 10)

	if got := selectedRow(t, m); got != 0 {
		t.Errorf("row %d is drawn as selected, want the root on row 0", got)
	}

	m = press(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

	if got := selectedRow(t, m); got != 1 {
		t.Errorf("row %d is drawn as selected after moving down, want row 1", got)
	}
}

// The band reaches the edge of the screen, so that the row is marked whatever
// it happens to hold.
func TestViewFillsTheCursorRow(t *testing.T) {
	const width = minWidth

	m := sized(t, openTestApp(t), width, 10)

	row := strings.Split(m.View().Content, "\n")[0]

	if got := lipgloss.Width(row); got != width {
		t.Errorf("the selected row is %d wide, want the full %d", got, width)
	}

	// The rows around it are left as they are, rather than padded as well.
	if got := lipgloss.Width(strings.Split(m.View().Content, "\n")[1]); got == width {
		t.Error("a row that is not selected was padded to the width of the screen")
	}
}

// The window follows the cursor, which is the application's decision; what is
// checked here is that the part drawn is the part it asked for.
func TestViewScrollsWithTheCursor(t *testing.T) {
	// Nine rows for the document, against a document of thirty-two.
	m := sized(t, openApp(t, longDocument(t)), minWidth, minHeight)

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") != "{" {
		t.Fatalf("row 0 = %q, want the top of the document", got)
	}

	// Down past the bottom of the window: the document scrolls, and the row
	// the cursor is on is still on screen.
	for range 12 {
		m = press(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	}

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") == "{" {
		t.Error("the window did not move, so the cursor has left the screen")
	}

	if row := selectedRow(t, m); row < 0 || row >= m.layout().BodyHeight {
		t.Errorf("the selected row is %d, outside the %d rows drawn", row, m.layout().BodyHeight)
	}

	// Back to the top, and so is the window.
	m = press(t, m, tea.KeyPressMsg{Code: 'g', Text: "g"}, tea.KeyPressMsg{Code: 'g', Text: "g"})

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") != "{" {
		t.Errorf("row 0 = %q, want the top of the document again", got)
	}
}

// A prefix that has been typed reaches the screen, not merely the model. What
// this catches that the tests either side of it do not is the bar being drawn
// from something other than what the key table left waiting: the key table
// answers with a prefix, the model stores it, and the bar renders whichever
// one it is handed, so only drawing a real frame joins the three.
func TestViewShowsAPendingPrefix(t *testing.T) {
	m := sized(t, openTestApp(t), 80, 12)

	bar := func(m Model) string {
		t.Helper()

		drawn := rows(t, m)

		return strings.TrimRight(drawn[len(drawn)-1], " ")
	}

	if got := bar(m); strings.HasSuffix(got, "  g") {
		t.Fatalf("the bar reads %q with nothing typed, want no prefix on it", got)
	}

	if got := bar(press(t, m, tea.KeyPressMsg{Code: 'g', Text: "g"})); !strings.HasSuffix(got, "  g") {
		t.Errorf("the bar reads %q after g, want it to end with the prefix", got)
	}

	if got := bar(press(t, m, tea.KeyPressMsg{Code: 'z', Text: "z"})); !strings.HasSuffix(got, "  z") {
		t.Errorf("the bar reads %q after z, want it to end with the prefix", got)
	}

	// Completing the sequence takes it off again.
	done := press(t, m,
		tea.KeyPressMsg{Code: 'g', Text: "g"},
		tea.KeyPressMsg{Code: 'g', Text: "g"},
	)

	if got := bar(done); strings.HasSuffix(got, "  g") {
		t.Errorf("the bar reads %q after gg, want the prefix gone", got)
	}
}

// The terminal only reports the wheel while it is asked to, and each frame is
// what asks.
func TestViewAsksForTheMouse(t *testing.T) {
	m := sized(t, openTestApp(t), 60, 10)

	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("MouseMode = %v, want %v", got, tea.MouseModeCellMotion)
	}

	// Turning it off gives the terminal its own text selection back, which is
	// the reason for the choice being a value rather than something fixed.
	m.mouse = false

	if got := m.View().MouseMode; got != tea.MouseModeNone {
		t.Errorf("MouseMode = %v with the mouse off, want %v", got, tea.MouseModeNone)
	}

	// The frame drawn before the size is known says the same thing.
	if got := NewModel(openTestApp(t), DefaultTheme()).View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("the first frame has MouseMode = %v, want %v", got, tea.MouseModeCellMotion)
	}
}

// However the terminal is shaped, the cursor is somewhere on it.
func TestViewKeepsTheCursorOnScreen(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: minWidth, height: minHeight},
		{width: minWidth, height: 12},
		{width: 120, height: minHeight},
		{width: 80, height: 24},
	}

	for _, size := range sizes {
		t.Run(strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
			m := sized(t, openApp(t, longDocument(t)), size.width, size.height)

			for range 20 {
				m = press(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

				if row := selectedRow(t, m); row < 0 || row >= m.layout().BodyHeight {
					t.Fatalf("the selected row is %d, outside the %d rows drawn", row, m.layout().BodyHeight)
				}
			}
		})
	}
}

// The document is laid out against the height of the terminal, which only
// works on a screen pino has to itself.
func TestViewUsesTheAlternateScreen(t *testing.T) {
	t.Parallel()

	app := openTestApp(t)

	if !NewModel(app, DefaultTheme()).View().AltScreen {
		t.Error("the first frame is not on the alternate screen")
	}

	if !sized(t, app, 60, 10).View().AltScreen {
		t.Error("the frame is not on the alternate screen")
	}
}

func TestEachViewUsesItsOwnIndentPolicy(t *testing.T) {
	tests := map[string]struct {
		view   application.ViewMode
		indent string
		want   string
	}{
		// The JSON view draws the whitespace the file will be saved with,
		// whatever that is.
		"the JSON view with two spaces":  {view: application.ViewJSON, indent: "  ", want: "  "},
		"the JSON view with four spaces": {view: application.ViewJSON, indent: "    ", want: "    "},
		"the JSON view with tabs":        {view: application.ViewJSON, indent: "\t", want: "\t"},
		"the JSON view with none":        {view: application.ViewJSON, indent: "", want: ""},

		// The tree view draws none of what is saved, so it uses a width of its
		// own: a tab-indented file would otherwise run off the screen, and one
		// written with eight spaces would spend forty columns on a depth of
		// five.
		"the tree view with two spaces":   {view: application.ViewTree, indent: "  ", want: "  "},
		"the tree view with tabs":         {view: application.ViewTree, indent: "\t", want: "  "},
		"the tree view with eight spaces": {view: application.ViewTree, indent: "        ", want: "  "},
		"the tree view with none":         {view: application.ViewTree, indent: "", want: "  "},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			info := application.StatusInfo{ViewMode: tc.view, Indent: tc.indent}

			if got := indentFor(info); got != tc.want {
				t.Errorf("indentFor(%+v) = %q, want %q", info, got, tc.want)
			}
		})
	}
}

// The JSON view has no pane beside it, so the band goes on reaching the edge
// of the screen there.
func TestViewFillsTheCursorRowInTheJSONView(t *testing.T) {
	const width = 120

	m := sized(t, openApp(t, nestedDocument(t)), width, 20)

	if got := lipgloss.Width(rows(t, m)[selectedRow(t, m)]); got != width {
		t.Errorf("the selected row is %d wide, want the full %d", got, width)
	}
}
