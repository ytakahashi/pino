package presentation

import (
	"errors"
	"slices"
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

// longDocument draws as thirty-two rows, comfortably more than the shortest
// terminal pino draws in can show, so that a window has somewhere to move to.
func longDocument(t *testing.T) domain.Node {
	t.Helper()

	members := make([]domain.Member, 0, 30)
	for i := range 30 {
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

// wideDocument holds a value long enough to overrun the narrowest terminal
// pino draws in, so that clipping has something to cut.
func wideDocument(t *testing.T) domain.Node {
	t.Helper()

	long, err := domain.NewString(strings.Repeat("x", 100))
	if err != nil {
		t.Fatalf("NewString() = %v", err)
	}

	root, err := domain.NewObject([]domain.Member{{Key: "long", Value: long}})
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
		JSONView: application.NewJSONRenderer(),
		TreeView: application.NewTreeRenderer(),
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
func TestViewWithoutADocument(t *testing.T) {
	app := application.New(application.Deps{
		Parser:   fakeParser{},
		Files:    fakeStore{},
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

// Resizing tells the application how much room the document has, which is
// what the window it asks for is worked out against.
func TestUpdateReportsTheBodyHeight(t *testing.T) {
	// Tall enough to reach well past what the shortest terminal could show,
	// without the window having had to move.
	m := sized(t, openApp(t, longDocument(t)), minWidth, 24)

	for range 15 {
		m = press(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	}

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") != "{" {
		t.Errorf("row 0 = %q, want the document still drawn from the top", got)
	}

	// Shrunk to less than the cursor's position, the window has to follow it.
	next, _ := m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})

	small, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if row := selectedRow(t, small); row < 0 || row >= small.layout().BodyHeight {
		t.Errorf("the selected row is %d, outside the %d rows drawn", row, small.layout().BodyHeight)
	}
}

// The wheel moves the window, and the selection stays on the screen.
func TestUpdateScrollsOnTheWheel(t *testing.T) {
	m := sized(t, openApp(t, longDocument(t)), minWidth, minHeight)

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

	if row := selectedRow(t, down); row < 0 || row >= down.layout().BodyHeight {
		t.Errorf("the selected row is %d, outside the %d rows drawn", row, down.layout().BodyHeight)
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
	m := sized(t, openApp(t, longDocument(t)), minWidth, minHeight)

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

	if !sized(t, app, 60, 10).View().AltScreen {
		t.Error("the frame is not on the alternate screen")
	}
}

// tab is the key that switches views.
var tabKey = tea.KeyPressMsg{Code: tea.KeyTab}

// openIndented is a session whose document was read from bytes laid out with
// indent, so that what the JSON view draws and what the bar reports follow the
// file rather than a default.
func openIndented(t *testing.T, root domain.Node, indent string) *application.App {
	t.Helper()

	app := application.New(application.Deps{
		Parser: fakeParser{root: root},
		Files: fakeStore{src: []byte(
			"{\n" + indent + "\"server\": {\n" + indent + indent + "\"host\": \"localhost\"\n" + indent + "}\n}\n",
		)},
		JSONView: application.NewJSONRenderer(),
		TreeView: application.NewTreeRenderer(),
	})

	if err := app.Open("config.json"); err != nil {
		t.Fatalf("Open() = %v", err)
	}

	return app
}

func TestIndentFor(t *testing.T) {
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

// bodyOf and paneOf are the two sides of an assembled row, the rule between
// them dropped.
func bodyOf(row string, l layout) string { return string([]rune(row)[:l.BodyWidth]) }

func paneOf(row string, l layout) string { return string([]rune(row)[l.BodyWidth+1:]) }

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
	want := []string{" Path      /", " Type      object", " Children  2", ""}

	for i, w := range want {
		if got := strings.TrimRight(drawn[l.BodyHeight+1+i], " "); got != w {
			t.Errorf("row %d of the pane is %q, want %q", i, got, w)
		}
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

// The session is told how much room the document has whenever that changes,
// and a view that brings an inspector with it changes it without the terminal
// having been touched.
func TestUpdateReportsTheHeightOnAViewSwitch(t *testing.T) {
	// Narrow enough for the inspector to go underneath, which is where it
	// costs the document rows.
	const width, height = 80, 20

	m := sized(t, openApp(t, tallDocument(t)), width, height)

	if got, want := m.reported, height-statusBarRows; got != want {
		t.Fatalf("the session was told %d rows, want %d", got, want)
	}

	m = press(t, m, tabKey)

	l := m.layout()
	if l.Inspector != placeBelow {
		t.Fatalf("the inspector is placed %v on a %d column terminal, want below", l.Inspector, width)
	}

	if got, want := m.reported, height-statusBarRows-l.InspectorHeight; got != want {
		t.Errorf("the session was told %d rows after Tab, want %d", got, want)
	}

	// The document is drawn in the rows it was told about, and the cursor is
	// among them.
	if row := selectedRow(t, m); row < 0 || row >= m.reported {
		t.Errorf("the selected row is %d, outside the %d rows the document has", row, m.reported)
	}

	// And back, which restores what the JSON view had.
	m = press(t, m, tabKey)

	if got, want := m.reported, height-statusBarRows; got != want {
		t.Errorf("the session was told %d rows after switching back, want %d", got, want)
	}
}

// Nothing is reported when the number has not moved. Saying it again would
// settle the session a second time for no reason on every key press.
func TestUpdateDoesNotRepeatTheHeight(t *testing.T) {
	// Wide enough that the inspector stands beside the tree, where it costs
	// columns rather than rows.
	m := sized(t, openApp(t, tallDocument(t)), 120, 20)

	before := m.reported

	next, cmd := m.Update(tabKey)
	if cmd != nil {
		t.Errorf("Update(tab) = %v, want no command; the height did not change", cmd)
	}

	after, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if after.reported != before {
		t.Errorf("the session was told %d rows, want the %d it already had", after.reported, before)
	}

	if after.layout().Inspector != placeSide {
		t.Errorf("the inspector is placed %v, want beside", after.layout().Inspector)
	}
}

// Below the size pino draws in, the screen says why rather than showing part
// of a document that cannot be arranged in the room left.
func TestViewSaysWhenTheTerminalIsTooSmall(t *testing.T) {
	got := rows(t, sized(t, openTestApp(t), 34, 6))

	if len(got) != 6 {
		t.Fatalf("View() drew %d rows, want 6", len(got))
	}

	want := []string{"terminal too small", "needs 60x10, has 34x6", "", "", "", ""}

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

// sizedFrom resizes a model that has already been drawn.
func sizedFrom(t *testing.T, m Model, width, height int) Model {
	t.Helper()

	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})

	model, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	return model
}
