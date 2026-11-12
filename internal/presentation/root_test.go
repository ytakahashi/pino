package presentation

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application"
	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/application/picker"
)

func TestRootSizesTheEditorAndCarriesMouseReportingIntoIt(t *testing.T) {
	t.Parallel()

	for name, mouse := range map[string]bool{"mouse enabled": true, "terminal selection": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m, _ := picking(t, "a.json", "b.json")
			m = updateRoot(t, m, tea.WindowSizeMsg{Width: 80, Height: 20}, key('j'))
			if !mouse {
				m = updateRoot(t, m, key('m'))
			}
			m = updateRoot(t, m, special(tea.KeyEnter))
			if m.editor == nil || m.picker != nil {
				t.Fatal("a successful open did not replace the picker with an editor")
			}
			if status := m.app.Status(); status.Name != "b.json" || status.New || status.Dirty {
				t.Errorf("editor status = %+v", status)
			}
			if !strings.Contains(m.View().Content, "localhost") {
				t.Error("the first editor frame does not show the selected document")
			}
			if got := m.View().MouseMode == tea.MouseModeCellMotion; got != mouse {
				t.Error("the editor did not inherit mouse reporting")
			}
			assertRootSize(t, m, 80, 20)
			m = updateRoot(t, m, tea.WindowSizeMsg{Width: 90, Height: 22}, key('m'))
			assertRootSize(t, m, 90, 22)
			if got := m.View().MouseMode == tea.MouseModeCellMotion; got == mouse {
				t.Error("the root did not forward the editor's mouse toggle")
			}
			_, cmd := m.Update(key('q'))
			if cmd == nil || !isQuit(cmd()) {
				t.Error("editor q did not quit the program")
			}
		})
	}
}

func TestRootReportsOpenFailuresAndAllowsAnotherSelection(t *testing.T) {
	t.Parallel()

	parseErr := errors.New("3:5: invalid value")
	for name, tc := range map[string]struct {
		readErr  error
		parseErr error
		want     error
	}{
		"parse failure": {parseErr: parseErr, want: parseErr},
		"missing file":  {readErr: &fs.PathError{Op: "open", Path: "configs/a.json", Err: fs.ErrNotExist}, want: fs.ErrNotExist},
		"read failure":  {readErr: fs.ErrPermission, want: fs.ErrPermission},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m, l := picking(t, "a.json", "b.json")
			files := &fakeFileStore{err: tc.readErr}
			parser := &fakeParser{root: testDocument(t), err: tc.parseErr}
			m.app = application.New(application.Deps{
				Parser: parser, Files: files,
				JSONView: documentview.NewJSONRenderer(), TreeView: documentview.NewTreeRenderer(),
			}, application.Config{})
			// The selected file disappeared from the next directory snapshot.
			l.entries = []picker.DirEntry{{Name: "b.json"}}
			m = updateRoot(t, m, special(tea.KeyEnter))
			if m.editor != nil || m.picker == nil {
				t.Fatal("a failed open left the picker")
			}
			f := m.picker.picker.Frame()
			if !errors.Is(f.Err, tc.want) || l.calls != 2 || len(f.Names) != 1 || f.Names[0] != "b.json" {
				t.Errorf("failure did not preserve its cause and refresh the names: %+v, lists=%d", f, l.calls)
			}
			// The directory is on the status row; the error row names the file.
			row := rootRows(m)[minHeight-2]
			if strings.Count(row, "a.json") != 1 || strings.Contains(row, "configs/") || !strings.Contains(row, tc.want.Error()) {
				t.Errorf("error row = %q, want the filename once and the cause", row)
			}
			if len(m.app.Frame().Lines) != 0 {
				t.Error("a failed open created a document")
			}
			assertRootSize(t, m, minWidth, minHeight)
			files.err, parser.err = nil, nil
			m = updateRoot(t, m, special(tea.KeyEnter))
			if m.editor == nil || m.app.Status().Name != "b.json" {
				t.Error("another selection did not open after the failure")
			}
		})
	}
}

func TestRootKeepsAnEmptyPickerAfterAFailedRefresh(t *testing.T) {
	t.Parallel()

	m, l := picking(t, "a.json")
	files := &fakeFileStore{err: fs.ErrNotExist}
	m.app = application.New(application.Deps{Files: files}, application.Config{})
	l.err = fs.ErrPermission
	m = updateRoot(t, m, special(tea.KeyEnter))
	if m.editor != nil || m.picker == nil || !strings.Contains(m.View().Content, "no .json files") {
		t.Fatal("the empty picker was not retained after listing failed")
	}
	if err := m.picker.picker.Frame().Err; !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("error = %v, want the open failure rather than the listing failure", err)
	}
	assertRootSize(t, m, minWidth, minHeight)
}

func TestRootWaitsForTheInitialTerminalSize(t *testing.T) {
	t.Parallel()

	m, _ := picking(t, "a.json")
	m = NewRootModel(m.app, m.picker.picker, m.theme)
	if m.Init() != nil || m.View().Content != "" || !m.View().AltScreen {
		t.Error("the root did not wait for its initial terminal size")
	}
	m = updateRoot(t, m, tea.WindowSizeMsg{Width: minWidth, Height: minHeight})
	if !strings.Contains(m.View().Content, "a.json") {
		t.Error("the initial resize did not reveal the picker")
	}
}
