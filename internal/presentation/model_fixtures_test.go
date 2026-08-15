package presentation

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// The doubles, documents and helpers the tests of this layer are written
// against. A model is worth little without a session behind it, so every file
// beside this one starts from something opened here.

// The doubles below stand in for the adapters the command line injects. The
// parser hands back a tree built by hand, so what is drawn here does not
// depend on parsing; the store hands back the bytes the layout is detected
// from.

type fakeFileStore struct{ src []byte }

func (s fakeFileStore) Read(string) ([]byte, application.Meta, error) { return s.src, nil, nil }

func (fakeFileStore) Write(string, []byte) (application.WriteOutcome, error) {
	return application.WriteOutcome{}, errors.ErrUnsupported
}

func (fakeFileStore) HasChangedSince(string, application.Meta) (application.ChangeStatus, error) {
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
		Files:    fakeFileStore{},
		JSONView: documentview.NewJSONRenderer(),
		TreeView: documentview.NewTreeRenderer(),
	}, application.Config{})

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

func isQuit(msg tea.Msg) bool {
	_, ok := msg.(tea.QuitMsg)

	return ok
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
		Files: fakeFileStore{src: []byte(
			"{\n" + indent + "\"server\": {\n" + indent + indent + "\"host\": \"localhost\"\n" + indent + "}\n}\n",
		)},
		JSONView: documentview.NewJSONRenderer(),
		TreeView: documentview.NewTreeRenderer(),
	}, application.Config{})

	if err := app.Open("config.json"); err != nil {
		t.Fatalf("Open() = %v", err)
	}

	return app
}

// bodyOf and paneOf are the two sides of an assembled row, the rule between
// them dropped.
func bodyOf(row string, l layout) string { return string([]rune(row)[:l.BodyWidth]) }

func paneOf(row string, l layout) string { return string([]rune(row)[l.BodyWidth+1:]) }

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
