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

// tallDocument draws as ten rows, more than the small terminals below can
// show, so that the window has somewhere to move to.
func tallDocument(t *testing.T) domain.Node {
	t.Helper()

	members := make([]domain.Member, 0, 8)
	for i := range 8 {
		members = append(members, domain.Member{
			Key:   "k" + strconv.Itoa(i),
			Value: domain.NewNumber(strconv.Itoa(i)),
		})
	}

	root, err := domain.NewObject(members)
	if err != nil {
		t.Fatalf("NewObject() = %v", err)
	}

	return root
}

// nestedDocument is deep enough that every way of moving has somewhere to go:
//
//	{
//	  "server": {
//	    "cache": {
//	      "ttl": 60
//	    },
//	    "host": "localhost"
//	  },
//	  "port": 8080
//	}
func nestedDocument(t *testing.T) domain.Node {
	t.Helper()

	host, err := domain.NewString("localhost")
	if err != nil {
		t.Fatalf("NewString() = %v", err)
	}

	cache, err := domain.NewObject([]domain.Member{{Key: "ttl", Value: domain.NewNumber("60")}})
	if err != nil {
		t.Fatalf("NewObject() = %v", err)
	}

	server, err := domain.NewObject([]domain.Member{
		{Key: "cache", Value: cache},
		{Key: "host", Value: host},
	})
	if err != nil {
		t.Fatalf("NewObject() = %v", err)
	}

	root, err := domain.NewObject([]domain.Member{
		{Key: "server", Value: server},
		{Key: "port", Value: domain.NewNumber("8080")},
	})
	if err != nil {
		t.Fatalf("NewObject() = %v", err)
	}

	return root
}

func openApp(t *testing.T, root domain.Node) *application.App {
	t.Helper()

	app := application.New(application.Deps{
		Parser:   fakeParser{root: root},
		Files:    fakeStore{},
		Renderer: application.NewJSONRenderer(),
	})

	if err := app.Open("config.json"); err != nil {
		t.Fatalf("Open() = %v", err)
	}

	return app
}

func openTestApp(t *testing.T) *application.App {
	t.Helper()

	return openApp(t, testDocument(t))
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

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd != nil {
		t.Errorf("Update(x) = %v, want no command", cmd)
	}

	assertSameSession(t, next, m)
}

// A prefix key produces nothing on its own and is remembered until the key
// that completes it arrives.
func TestUpdateRemembersAPrefix(t *testing.T) {
	m := sized(t, openTestApp(t), 80, 24)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if cmd != nil {
		t.Errorf("Update(g) = %v, want no command", cmd)
	}

	after, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if after.pending != PendingG {
		t.Errorf("pending = %v after g, want %v", after.pending, PendingG)
	}

	// The second g completes it and is acted on.
	next, _ = after.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})

	done, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if done.pending != PendingNone {
		t.Errorf("pending = %v after gg, want nothing waiting", done.pending)
	}
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

	if got.app != m.app || got.width != m.width || got.height != m.height || got.pending != m.pending {
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

	// The bar is anchored to the last row, whatever the document is worth,
	// with where the selection is at one end and what the document is at the
	// other.
	bar := got[7]

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

	// Nothing is selected, so the bar says only what is true of the session.
	bar := got[3]

	if !strings.HasPrefix(bar, " NORMAL  JSON") {
		t.Errorf("the bar begins %q, want the mode and the view", bar)
	}

	if !strings.HasSuffix(bar, "0 lines  indent:2 ") {
		t.Errorf("the bar ends %q, want the state of the document", bar)
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

// press sends key presses in order, answering the model they leave behind.
func press(t *testing.T, m Model, keys ...tea.KeyPressMsg) Model {
	t.Helper()

	for _, k := range keys {
		next, _ := m.Update(k)

		got, ok := next.(Model)
		if !ok {
			t.Fatalf("Update() returned %T, want Model", next)
		}

		m = got
	}

	return m
}

// selectedRow is the row drawn with the cursor's own styling, or -1. It is
// found by its background, which nothing else on a body row carries.
func selectedRow(t *testing.T, m Model) int {
	t.Helper()

	marker := cursorBackground(t, m.theme)
	found := -1

	for i, row := range strings.Split(m.View().Content, "\n") {
		if !strings.Contains(row, marker) {
			continue
		}

		if found >= 0 {
			t.Fatalf("rows %d and %d are both drawn as selected", found, i)
		}

		found = i
	}

	return found
}

// The row the cursor is on is the one marked, and moving moves the mark.
func TestViewMarksTheCursorRow(t *testing.T) {
	m := sized(t, openTestApp(t), 40, 8)

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
	const width = 40

	m := sized(t, openTestApp(t), width, 8)

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
	// Three rows for the document, against a document of ten.
	m := sized(t, openApp(t, tallDocument(t)), 40, 4)

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") != "{" {
		t.Fatalf("row 0 = %q, want the top of the document", got)
	}

	// Down past the bottom of the window: the document scrolls, and the row
	// the cursor is on is still on screen.
	for range 5 {
		m = press(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	}

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") == "{" {
		t.Error("the window did not move, so the cursor has left the screen")
	}

	if row := selectedRow(t, m); row < 0 || row >= m.bodyHeight() {
		t.Errorf("the selected row is %d, outside the %d rows drawn", row, m.bodyHeight())
	}

	// Back to the top, and so is the window.
	m = press(t, m, tea.KeyPressMsg{Code: 'g', Text: "g"}, tea.KeyPressMsg{Code: 'g', Text: "g"})

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") != "{" {
		t.Errorf("row 0 = %q, want the top of the document again", got)
	}
}

// Resizing tells the application how much room the document has, which is
// what the window it asks for is worked out against.
func TestUpdateReportsTheBodyHeight(t *testing.T) {
	// Tall enough for the whole document, so nothing is scrolled.
	m := sized(t, openApp(t, tallDocument(t)), 40, 24)

	for range 5 {
		m = press(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	}

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") != "{" {
		t.Errorf("row 0 = %q, want the document still drawn from the top", got)
	}

	// Shrunk to less than the cursor's position, the window has to follow it.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 3})

	small, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if row := selectedRow(t, small); row < 0 || row >= small.bodyHeight() {
		t.Errorf("the selected row is %d, outside the %d rows drawn", row, small.bodyHeight())
	}
}

// The wheel moves the window, and the selection stays on the screen.
func TestUpdateScrollsOnTheWheel(t *testing.T) {
	m := sized(t, openApp(t, tallDocument(t)), 40, 5)

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") != "{" {
		t.Fatalf("row 0 = %q, want the top of the document", got)
	}

	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})

	down, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if got := rows(t, down)[0]; strings.TrimRight(got, " ") == "{" {
		t.Error("the wheel did not move the window")
	}

	if row := selectedRow(t, down); row < 0 || row >= down.bodyHeight() {
		t.Errorf("the selected row is %d, outside the %d rows drawn", row, down.bodyHeight())
	}

	// And back up again.
	next, _ = down.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})

	up, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if got := rows(t, up)[0]; strings.TrimRight(got, " ") != "{" {
		t.Errorf("row 0 = %q, want the top of the document again", got)
	}
}

// Sideways there is nothing to scroll: a row too wide for the screen is cut,
// not moved.
func TestUpdateIgnoresTheHorizontalWheel(t *testing.T) {
	m := sized(t, openApp(t, tallDocument(t)), 40, 5)

	next, cmd := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelRight})
	if cmd != nil {
		t.Errorf("Update() = %v, want no command", cmd)
	}

	assertSameSession(t, next, m)

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") != "{" {
		t.Errorf("row 0 = %q, want the window left where it was", got)
	}
}

// A prefix that has been typed reaches the screen, not merely the model. What
// this catches that the tests either side of it do not is the bar being drawn
// from something other than what the key table left waiting: the key table
// answers with a prefix, the model stores it, and the bar renders whichever
// one it is handed, so only drawing a real frame joins the three.
func TestViewShowsAPendingPrefix(t *testing.T) {
	m := sized(t, openTestApp(t), 80, 8)

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
	m := sized(t, openTestApp(t), 40, 8)

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
		{width: 40, height: 3},
		{width: 40, height: 4},
		{width: 12, height: 5},
		{width: 80, height: 24},
	}

	for _, size := range sizes {
		t.Run(strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
			m := sized(t, openApp(t, tallDocument(t)), size.width, size.height)

			for range 9 {
				m = press(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

				if row := selectedRow(t, m); row < 0 || row >= m.bodyHeight() {
					t.Fatalf("the selected row is %d, outside the %d rows drawn", row, m.bodyHeight())
				}
			}
		})
	}
}

func isQuit(msg tea.Msg) bool {
	_, ok := msg.(tea.QuitMsg)

	return ok
}

// The document is laid out against the height of the terminal, which only
// works on a screen pino has to itself.
func TestViewUsesTheAlternateScreen(t *testing.T) {
	t.Parallel()

	app := openTestApp(t)

	if !NewModel(app, DefaultTheme()).View().AltScreen {
		t.Error("the first frame is not on the alternate screen")
	}

	if !sized(t, app, 40, 10).View().AltScreen {
		t.Error("the frame is not on the alternate screen")
	}
}
