package presentation

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application/picker"
)

func TestPickerDrawsTheNamesSelectionAndCounts(t *testing.T) {
	t.Parallel()

	m, _ := picking(t, "b.json", "a.json")
	rows := rootRows(m)
	if strings.TrimSpace(rows[0]) != "a.json" || strings.TrimSpace(rows[1]) != "b.json" {
		t.Fatalf("first rows = %q, want sorted names", rows[:2])
	}
	if !strings.Contains(rows[len(rows)-1], "configs") || !strings.Contains(rows[len(rows)-1], "2/2") {
		t.Errorf("status = %q, want directory and counts", rows[len(rows)-1])
	}
	marker := cursorBackground(t, m.theme)
	styled := strings.Split(m.View().Content, "\n")
	if !strings.Contains(styled[0], marker) || strings.Contains(styled[1], marker) {
		t.Error("the first file is not the only highlighted name")
	}
	m = updateRoot(t, m, key('j'))
	styled = strings.Split(m.View().Content, "\n")
	if strings.Contains(styled[0], marker) || !strings.Contains(styled[1], marker) {
		t.Error("the highlight did not follow the cursor")
	}
	assertRootSize(t, m, minWidth, minHeight)
}

func TestPickerEscapesNamesAndShortensWideNames(t *testing.T) {
	t.Parallel()

	wide := strings.Repeat("界", minWidth) + ".json"
	for name, tc := range map[string]struct {
		file string
		want func(row string) bool
	}{
		"control characters": {
			file: "a\nb\t\x1b[31m.json",
			want: func(row string) bool { return row == `a\nb\t\x1b[31m.json` },
		},
		"quotes and backslashes": {
			file: `a"b\c.json`,
			want: func(row string) bool { return row == `a"b\c.json` },
		},
		"a name wider than the screen": {
			file: wide,
			want: func(row string) bool { return strings.HasSuffix(row, "…") },
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m, _ := picking(t, tc.file)
			if row := strings.TrimRight(rootRows(m)[0], " "); !tc.want(row) {
				t.Errorf("row = %q, drawn from %q", row, tc.file)
			}
			assertRootSize(t, m, minWidth, minHeight)

			m = updateRoot(t, m, special(tea.KeyEnter))
			if m.editor == nil || m.app.Status().Name != tc.file {
				t.Error("opening the file did not preserve its raw name")
			}
		})
	}
}

func TestPickerFiltersWhileTypingAndRestoresTheSelectionOnCancel(t *testing.T) {
	t.Parallel()

	m, _ := picking(t, "a.json", "b.json", "c.json")
	m = updateRoot(t, m, key('j'), key('/'), key('c'))
	f := m.picker.picker.Frame()
	if f.Filter != "c" || !reflect.DeepEqual(f.Names, []string{"c.json"}) {
		t.Fatalf("typed filter frame = %+v", f)
	}
	if m.picker.picker.Mode() != picker.ModeFilter || !strings.HasPrefix(rootRows(m)[minHeight-2], "/c") {
		t.Error("filter input is not shown")
	}
	assertRootSize(t, m, minWidth, minHeight)
	m = updateRoot(t, m, special(tea.KeyEscape))
	if f = m.picker.picker.Frame(); f.Filter != "" || f.Cursor != 1 || len(f.Names) != 3 {
		t.Errorf("cancelled filter frame = %+v, want the original selection", f)
	}

	m = updateRoot(t, m, key('/'), key('b'), special(tea.KeyEnter))
	if m.editor != nil || m.picker.picker.Mode() != picker.ModeNormal {
		t.Fatal("submitting the filter opened a file")
	}
	if bar := rootRows(m)[minHeight-1]; !strings.Contains(bar, "/b") || !strings.Contains(bar, "1/3") {
		t.Errorf("filtered status = %q", bar)
	}
	m = updateRoot(t, m, key('/'))
	if m.picker.editor.Value() != "b" {
		t.Error("filter input did not start with the accepted term")
	}
	m = updateRoot(t, m, special(tea.KeyEnter), special(tea.KeyEscape))
	if f = m.picker.picker.Frame(); f.Filter != "" || f.Cursor != 1 || len(f.Names) != 3 {
		t.Errorf("cleared filter frame = %+v, want b.json still selected", f)
	}
}

func TestPickerPastesIntoTheFilterAndShowsNoMatches(t *testing.T) {
	t.Parallel()

	m, _ := picking(t, "a.json")
	m = updateRoot(t, m, tea.PasteMsg{Content: "ignored"}, key('/'), tea.PasteMsg{Content: "missing"})
	if m.picker.editor.Value() != "missing" || strings.TrimSpace(rootRows(m)[0]) != "no matches" {
		t.Errorf("paste did not narrow the list: %q", rootRows(m)[0])
	}
	m = updateRoot(t, m, special(tea.KeyEnter), special(tea.KeyEnter))
	if m.editor != nil || m.picker == nil {
		t.Error("Enter on no matches opened an editor")
	}
}

func TestPickerScrollsAndKeepsTheSelectedNameVisible(t *testing.T) {
	t.Parallel()

	names := make([]string, 30)
	for i := range names {
		names[i] = fmt.Sprintf("%02d.json", i)
	}
	m, _ := picking(t, names...)
	m = updateRoot(t, m, key('G'))
	f := m.picker.picker.Frame()
	if f.Cursor != 29 || !strings.Contains(strings.Join(rootRows(m)[:minHeight-1], "\n"), "29.json") {
		t.Errorf("last name is not visible: %+v", f)
	}
	m = updateRoot(t, m, key('g'), key('g'), tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	f = m.picker.picker.Frame()
	if f.Scroll != wheelRows || f.Cursor < f.Scroll || f.Cursor >= f.Scroll+minHeight-1 {
		t.Errorf("wheel frame = %+v", f)
	}
	m = updateRoot(t, m, key('m'))
	before := m.picker.picker.Frame()
	m = updateRoot(t, m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if !reflect.DeepEqual(m.picker.picker.Frame(), before) {
		t.Error("wheel moved the list after mouse reporting was turned off")
	}
}

func TestPickerShowsErrorsUntilTheNextKeyAndKeepsThemOnResize(t *testing.T) {
	t.Parallel()

	for name, msg := range map[string]tea.Msg{"unbound": key('x'), "prefix": key('g'), "terminal": key('m'), "move": key('j')} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m, _ := picking(t, "a.json")
			m.picker.picker.Failed(errors.New("bad\nfile\x1b"))
			m = updateRoot(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
			if row := rootRows(m)[18]; !strings.Contains(row, `bad\nfile\x1b`) {
				t.Errorf("error row after resize = %q", row)
			}
			assertRootSize(t, m, 80, 20)
			m = updateRoot(t, m, msg)
			if m.picker.picker.Frame().Err != nil || strings.Contains(m.View().Content, "bad") {
				t.Error("the next key did not dismiss the error")
			}
		})
	}
}

func TestPickerIgnoresListKeysWhileHelpIsShown(t *testing.T) {
	t.Parallel()

	m, _ := picking(t, "a.json", "b.json")
	m = updateRoot(t, m, key('?'))
	if !strings.Contains(m.View().Content, "pino file picker") {
		t.Fatal("help is not shown")
	}
	before := m.picker.picker.Frame()
	m = updateRoot(t, m, key('j'), key('m'), special(tea.KeyEnter), tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.editor != nil || !reflect.DeepEqual(before, m.picker.picker.Frame()) || m.View().MouseMode != tea.MouseModeCellMotion {
		t.Error("a key or wheel event acted through help")
	}
	for _, k := range []tea.KeyPressMsg{key('?'), key('q'), special(tea.KeyEscape)} {
		after := updateRoot(t, m, k)
		if after.picker.picker.Mode() != picker.ModeNormal {
			t.Errorf("%q did not close help", k.String())
		}
		m = updateRoot(t, after, key('?'))
	}
}

func TestPickerTooSmallStillAllowsQuittingAndRecoversOnResize(t *testing.T) {
	t.Parallel()

	m, _ := picking(t, "a.json")
	m = updateRoot(t, m, tea.WindowSizeMsg{Width: 34, Height: 6})
	if rootRows(m)[0] != "terminal too small" {
		t.Fatal("small terminal warning is not shown")
	}
	assertRootSize(t, m, 34, 6)
	_, cmd := m.Update(key('q'))
	if cmd == nil || !isQuit(cmd()) {
		t.Fatal("q did not quit behind the warning")
	}
	m = updateRoot(t, m, tea.WindowSizeMsg{Width: minWidth, Height: minHeight})
	if strings.TrimSpace(rootRows(m)[0]) != "a.json" {
		t.Error("the list did not recover on resize")
	}
}
