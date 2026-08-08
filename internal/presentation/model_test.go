package presentation

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
	"github.com/ytakahashi/pino/internal/domain"
)

// The doubles below stand in for the adapters the command line injects. The
// parser hands back a tree built by hand, so what is drawn here does not
// depend on parsing; the store hands back the bytes the layout is detected
// from.

type fakeStore struct{ src []byte }

func (s fakeStore) Read(string) ([]byte, application.Meta, error) { return s.src, nil, nil }

func (fakeStore) Write(string, []byte) error { return errors.ErrUnsupported }

func (fakeStore) HasChangedSince(string, application.Meta) (application.ChangeStatus, error) {
	return application.ChangeModified, errors.ErrUnsupported
}

type fakeParser struct{ root domain.Node }

func (p fakeParser) Parse([]byte, domain.Dialect) (domain.Node, error) { return p.root, nil }

// testDocument draws as four rows:
//
//	{
//	  "host": "localhost",
//	  "port": 8080
//	}
func testDocument(t *testing.T) domain.Node {
	t.Helper()

	host, err := domain.NewString("localhost")
	if err != nil {
		t.Fatalf("NewString() = %v", err)
	}

	root, err := domain.NewObject([]domain.Member{
		{Key: "host", Value: host},
		{Key: "port", Value: domain.NewNumber("8080")},
	})
	if err != nil {
		t.Fatalf("NewObject() = %v", err)
	}

	return root
}

func openTestApp(t *testing.T) *application.App {
	t.Helper()

	app := application.New(application.Deps{
		Parser:   fakeParser{root: testDocument(t)},
		Files:    fakeStore{},
		Renderer: application.NewJSONRenderer(),
	})

	if err := app.Open("config.json"); err != nil {
		t.Fatalf("Open() = %v", err)
	}

	return app
}

// sized is a model that has already been told how big the terminal is.
func sized(t *testing.T, app *application.App, width, height int) Model {
	t.Helper()

	next, _ := NewModel(app, DefaultTheme()).Update(tea.WindowSizeMsg{Width: width, Height: height})

	model, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	return model
}

// rows is what View draws, without styling, one entry per row.
func rows(t *testing.T, m Model) []string {
	t.Helper()

	return strings.Split(ansi.Strip(m.View().Content), "\n")
}

func TestUpdateStoresWindowSize(t *testing.T) {
	m := sized(t, openTestApp(t), 80, 24)

	if m.width != 80 || m.height != 24 {
		t.Errorf("size = %dx%d, want 80x24", m.width, m.height)
	}
}

func TestUpdateQuitsOnBoundKey(t *testing.T) {
	m := sized(t, openTestApp(t), 80, 24)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("Update(q) returned no command, want a quit")
	}

	if msg := cmd(); !isQuit(msg) {
		t.Errorf("Update(q) command produced %T, want tea.QuitMsg", msg)
	}
}

// A key nothing is bound to leaves the session as it was. Resolving it to an
// Action the application then ignores would be the same outcome, but this way
// the application is never asked about a key press that means nothing.
func TestUpdateIgnoresUnboundKey(t *testing.T) {
	m := sized(t, openTestApp(t), 80, 24)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if cmd != nil {
		t.Errorf("Update(z) = %v, want no command", cmd)
	}

	assertSameSession(t, next, m)
}

func TestUpdateIgnoresUnknownMessage(t *testing.T) {
	m := sized(t, openTestApp(t), 80, 24)

	next, cmd := m.Update(tea.KeyReleaseMsg{Code: 'q', Text: "q"})
	if cmd != nil {
		t.Errorf("Update() = %v, want no command", cmd)
	}

	assertSameSession(t, next, m)
}

// A Model holds styles, which are not comparable, so what is checked is that
// nothing the model is responsible for has moved.
func assertSameSession(t *testing.T, next tea.Model, m Model) {
	t.Helper()

	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if got.app != m.app || got.width != m.width || got.height != m.height {
		t.Errorf("Update() changed the model")
	}
}

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
	m := sized(t, openTestApp(t), 60, 8)

	got := rows(t, m)
	if len(got) != 8 {
		t.Fatalf("View() drew %d rows, want 8", len(got))
	}

	want := []string{
		"{",
		`  "host": "localhost",`,
		`  "port": 8080`,
		"}",
		"", "", "",
	}

	for i, w := range want {
		if strings.TrimRight(got[i], " ") != w {
			t.Errorf("row %d = %q, want %q", i, got[i], w)
		}
	}

	// The bar is anchored to the last row, whatever the document is worth.
	if bar := got[7]; !strings.HasPrefix(bar, " NORMAL  JSON  config.json  4 lines  indent:2") {
		t.Errorf("last row = %q, want the status bar", bar)
	}
}

// A document taller than the screen is shown from the top and cut off. It is
// the whole document that is counted in the bar, not the part on screen.
func TestViewCutsTheDocumentToTheBodyHeight(t *testing.T) {
	m := sized(t, openTestApp(t), 40, 3)

	got := rows(t, m)
	if len(got) != 3 {
		t.Fatalf("View() drew %d rows, want 3", len(got))
	}

	if want := "{"; strings.TrimRight(got[0], " ") != want {
		t.Errorf("row 0 = %q, want %q", got[0], want)
	}

	if want := `  "host": "localhost",`; strings.TrimRight(got[1], " ") != want {
		t.Errorf("row 1 = %q, want %q", got[1], want)
	}

	if !strings.Contains(got[2], "4 lines") {
		t.Errorf("status bar = %q, want the whole document counted", got[2])
	}
}

// Every row is at most as wide as the terminal, so that a long line takes one
// row rather than wrapping onto the next and displacing everything below it.
func TestViewClipsRowsToTheWidth(t *testing.T) {
	const width = 12

	m := sized(t, openTestApp(t), width, 8)

	for i, row := range rows(t, m) {
		if w := lipgloss.Width(row); w > width {
			t.Errorf("row %d is %d wide, want at most %d: %q", i, w, width, row)
		}
	}
}

// Nothing is open until a document has been read, and drawing has to survive
// that: the program is on the screen before the first file arrives.
func TestViewWithoutADocument(t *testing.T) {
	app := application.New(application.Deps{
		Parser:   fakeParser{},
		Files:    fakeStore{},
		Renderer: application.NewJSONRenderer(),
	})

	got := rows(t, sized(t, app, 40, 4))
	if len(got) != 4 {
		t.Fatalf("View() drew %d rows, want 4", len(got))
	}

	for i, row := range got[:3] {
		if strings.TrimRight(row, " ") != "" {
			t.Errorf("row %d = %q, want it blank", i, row)
		}
	}

	if want := " NORMAL  JSON  0 lines  indent:2"; !strings.HasPrefix(got[3], want) {
		t.Errorf("status bar = %q, want prefix %q", got[3], want)
	}
}

// A terminal too small to be useful is still a terminal pino is drawn on.
// Telling the reader that it is too small belongs with the rest of the
// responsive layout, which arrives with the tree view; until then the screen
// is filled as far as it goes, and what this rules out is drawing outside it.
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

func isQuit(msg tea.Msg) bool {
	_, ok := msg.(tea.QuitMsg)

	return ok
}
