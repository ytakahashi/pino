package presentation

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/application/picker"
)

type fakeDirLister struct {
	entries []picker.DirEntry
	err     error
	calls   int
}

func (l *fakeDirLister) List(string) ([]picker.DirEntry, error) {
	l.calls++

	return l.entries, l.err
}

func picking(t *testing.T, names ...string) (RootModel, *fakeDirLister) {
	t.Helper()

	l := &fakeDirLister{}
	for _, name := range names {
		l.entries = append(l.entries, picker.DirEntry{Name: name})
	}
	p, err := picker.New(l, "configs")
	if err != nil {
		t.Fatalf("picker.New: %v", err)
	}

	app := application.New(application.Deps{
		Parser:   fakeParser{root: testDocument(t)},
		Files:    fakeFileStore{},
		JSONView: documentview.NewJSONRenderer(),
		TreeView: documentview.NewTreeRenderer(),
	}, application.Config{})

	return updateRoot(t, NewRootModel(app, p, DefaultTheme()), tea.WindowSizeMsg{Width: minWidth, Height: minHeight}), l
}

func updateRoot(t *testing.T, m RootModel, messages ...tea.Msg) RootModel {
	t.Helper()

	for _, msg := range messages {
		next, _ := m.Update(msg)
		var ok bool
		m, ok = next.(RootModel)
		if !ok {
			t.Fatalf("Update returned %T, want RootModel", next)
		}
	}

	return m
}

func rootRows(m RootModel) []string {
	return strings.Split(ansi.Strip(m.View().Content), "\n")
}

func assertRootSize(t *testing.T, m RootModel, width, height int) {
	t.Helper()

	rows := rootRows(m)
	if len(rows) != height {
		t.Fatalf("View drew %d rows, want %d", len(rows), height)
	}
	for i, row := range rows {
		if got := ansi.StringWidth(row); got > width {
			t.Errorf("row %d is %d columns, want at most %d: %q", i, got, width, row)
		}
	}
}

func pickerBindingFor(t *testing.T, act picker.Action) pickerBinding {
	t.Helper()

	for _, b := range pickerBindings {
		if b.Action == act {
			return b
		}
	}

	t.Fatalf("no picker binding for %T", act)

	return pickerBinding{}
}

func normalBindingFor(t *testing.T, act application.Action) binding {
	t.Helper()

	for _, b := range normalBindings {
		if b.Action == act {
			return b
		}
	}

	t.Fatalf("no editor binding for %T", act)

	return binding{}
}
