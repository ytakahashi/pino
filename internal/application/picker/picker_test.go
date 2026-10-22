package picker

import (
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"
)

// Which names are offered, and in what order.

func TestCandidatesAreJSONFilesInNameOrder(t *testing.T) {
	t.Parallel()

	l := &fakeDirLister{results: []listResult{{entries: []DirEntry{
		{Name: "b.json"},
		{Name: "notes.txt"},
		{Name: "A.JSON"},
		{Name: "dir.json", Dir: true},
		{Name: ".eslintrc.json"},
		{Name: "x.jsonc"},
		{Name: "json"},
	}}}}

	got := newPicker(t, l, "dir").Frame()

	want := []string{".eslintrc.json", "A.JSON", "b.json"}
	if !reflect.DeepEqual(got.Names, want) {
		t.Errorf("Names = %q, want %q", got.Names, want)
	}

	if got.Total != len(want) {
		t.Errorf("Total = %d, want %d", got.Total, len(want))
	}

	if got.Cursor != 0 {
		t.Errorf("Cursor = %d, want 0", got.Cursor)
	}

	if !reflect.DeepEqual(l.dirs, []string{"dir"}) {
		t.Errorf("listed %q, want [dir]", l.dirs)
	}
}

func TestNewReturnsTheListingError(t *testing.T) {
	t.Parallel()

	l := &fakeDirLister{results: []listResult{{err: fs.ErrPermission}}}

	if _, err := New(l, "dir"); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("New error = %v, want %v", err, fs.ErrPermission)
	}
}

func TestEmptyReportsADirectoryWithNoCandidates(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		names []string
		want  bool
	}{
		"nothing listed":     {want: true},
		"no JSON file":       {names: []string{"notes.txt"}, want: true},
		"one JSON file":      {names: []string{"notes.txt", "a.json"}},
		"several JSON files": {names: []string{"a.json", "b.json"}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := newPicker(t, listing(tc.names...), "dir")
			if got := p.Empty(); got != tc.want {
				t.Errorf("Empty() = %t, want %t", got, tc.want)
			}

			if tc.want && p.Frame().Cursor != -1 {
				t.Errorf("Cursor = %d with nothing to pick, want -1", p.Frame().Cursor)
			}
		})
	}
}

// Moving the selection and the window.

func TestMovesStopAtTheEnds(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		acts []Action
		want int
	}{
		"next":                {acts: []Action{ActionMoveNext{}}, want: 1},
		"previous at the top": {acts: []Action{ActionMovePrev{}}, want: 0},
		"last":                {acts: []Action{ActionMoveLast{}}, want: 2},
		"next at the bottom":  {acts: []Action{ActionMoveLast{}, ActionMoveNext{}}, want: 2},
		"previous":            {acts: []Action{ActionMoveLast{}, ActionMovePrev{}}, want: 1},
		"first":               {acts: []Action{ActionMoveLast{}, ActionMoveFirst{}}, want: 0},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := newPicker(t, listing(numbered(3)...), "dir")
			for _, act := range tc.acts {
				p.Do(act)
			}

			if got := p.Frame().Cursor; got != tc.want {
				t.Errorf("Cursor = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMovingKeepsTheSelectionInTheWindow(t *testing.T) {
	t.Parallel()

	p := newPicker(t, listing(numbered(10)...), "dir")
	p.SetSize(3)

	steps := []struct {
		act            Action
		cursor, scroll int
	}{
		{ActionMoveLast{}, 9, 7},
		{ActionMovePrev{}, 8, 7},
		{ActionMoveFirst{}, 0, 0},
		{ActionMoveNext{}, 1, 0},
		{ActionMoveNext{}, 2, 0},
		{ActionMoveNext{}, 3, 1},
	}

	for _, s := range steps {
		p.Do(s.act)

		f := p.Frame()
		if f.Cursor != s.cursor || f.Scroll != s.scroll {
			t.Fatalf("after %T: Cursor, Scroll = %d, %d, want %d, %d",
				s.act, f.Cursor, f.Scroll, s.cursor, s.scroll)
		}
	}
}

func TestScrollHalfMovesTheWindowAndTheSelectionTogether(t *testing.T) {
	t.Parallel()

	p := newPicker(t, listing(numbered(10)...), "dir")
	p.SetSize(4)
	p.Do(ActionMoveNext{})

	steps := []struct {
		act            Action
		cursor, scroll int
	}{
		{ActionScrollHalfDown{}, 3, 2},
		{ActionScrollHalfDown{}, 5, 4},
		// The window stops at the end, and the selection goes on to it.
		{ActionScrollHalfDown{}, 7, 6},
		{ActionScrollHalfDown{}, 9, 6},
		{ActionScrollHalfUp{}, 7, 4},
		{ActionScrollHalfUp{}, 5, 2},
		{ActionScrollHalfUp{}, 3, 0},
		{ActionScrollHalfUp{}, 1, 0},
	}

	for _, s := range steps {
		p.Do(s.act)

		f := p.Frame()
		if f.Cursor != s.cursor || f.Scroll != s.scroll {
			t.Fatalf("after %T: Cursor, Scroll = %d, %d, want %d, %d",
				s.act, f.Cursor, f.Scroll, s.cursor, s.scroll)
		}
	}
}

func TestScrollByTakesTheSelectionOnlyAsFarAsTheWindow(t *testing.T) {
	t.Parallel()

	p := newPicker(t, listing(numbered(10)...), "dir")
	p.SetSize(3)

	steps := []struct {
		rows           int
		cursor, scroll int
	}{
		// The selection is pushed down by the top edge of the window.
		{4, 4, 4},
		// It is still in the window, so it stays where it is.
		{-2, 4, 2},
		// And pushed up by the bottom edge.
		{-2, 2, 0},
		// The window stops at the end.
		{100, 7, 7},
	}

	for _, s := range steps {
		p.Do(ActionScrollBy{Rows: s.rows})

		f := p.Frame()
		if f.Cursor != s.cursor || f.Scroll != s.scroll {
			t.Fatalf("after scrolling %d: Cursor, Scroll = %d, %d, want %d, %d",
				s.rows, f.Cursor, f.Scroll, s.cursor, s.scroll)
		}
	}
}

func TestShrinkingTheWindowKeepsTheSelectionInIt(t *testing.T) {
	t.Parallel()

	p := newPicker(t, listing(numbered(10)...), "dir")
	p.SetSize(10)
	p.Do(ActionMoveLast{})
	p.SetSize(3)

	if f := p.Frame(); f.Scroll != 7 {
		t.Errorf("Scroll = %d, want 7", f.Scroll)
	}
}

// Narrowing the list.

func TestFilterNarrowsTheListAsTheTermIsTyped(t *testing.T) {
	t.Parallel()

	p := newPicker(t, listing("app.json", "config.json", "config.local.json", "package.json"), "dir")
	p.Do(ActionMoveLast{})

	effects := p.Do(ActionBeginFilter{})
	if !reflect.DeepEqual(effects, []Effect{EffectBeginInput{}}) {
		t.Errorf("BeginFilter effects = %#v, want an empty input", effects)
	}

	if p.Mode() != ModeFilter {
		t.Fatalf("Mode = %d, want ModeFilter", p.Mode())
	}

	p.Do(ActionFilterChange{Text: "con"})
	p.Do(ActionMoveFirst{}) // not a key of the box being typed in
	p.Do(ActionFilterChange{Text: "conf"})

	f := p.Frame()

	want := []string{"config.json", "config.local.json"}
	if !reflect.DeepEqual(f.Names, want) {
		t.Errorf("Names = %q, want %q", f.Names, want)
	}

	if f.Cursor != 0 {
		t.Errorf("Cursor = %d, want the first match", f.Cursor)
	}

	if f.Filter != "conf" || f.Total != 4 {
		t.Errorf("Filter, Total = %q, %d, want %q, 4", f.Filter, f.Total, "conf")
	}
}

func TestFilterMatchesWithSmartcase(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		term string
		want []string
	}{
		"lowercase ignores case": {term: "conf", want: []string{"Config.json", "config.json"}},
		"uppercase keeps case":   {term: "Conf", want: []string{"Config.json"}},
		"empty term keeps all":   {term: "", want: []string{"Config.json", "app.json", "config.json"}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := newPicker(t, listing("app.json", "config.json", "Config.json"), "dir")
			p.Do(ActionBeginFilter{})
			p.Do(ActionFilterChange{Text: tc.term})

			if got := p.Frame().Names; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Names = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFilterSubmitKeepsTheTerm(t *testing.T) {
	t.Parallel()

	p := newPicker(t, listing("app.json", "config.json", "config.local.json"), "dir")
	p.Do(ActionBeginFilter{})
	p.Do(ActionFilterChange{Text: "conf"})
	p.Do(ActionFilterSubmit{Text: "conf"})

	if p.Mode() != ModeNormal {
		t.Errorf("Mode = %d, want ModeNormal", p.Mode())
	}

	f := p.Frame()
	if f.Filter != "conf" || len(f.Names) != 2 {
		t.Errorf("Filter, Names = %q, %q, want the two conf files", f.Filter, f.Names)
	}

	// The list is back to taking moves.
	p.Do(ActionMoveNext{})

	if got := selected(p.Frame()); got != "config.local.json" {
		t.Errorf("selected %q, want config.local.json", got)
	}
}

func TestBeginFilterStartsFromTheTermInEffect(t *testing.T) {
	t.Parallel()

	p := newPicker(t, listing("app.json", "config.json", "config.local.json"), "dir")
	p.Do(ActionBeginFilter{})
	p.Do(ActionFilterSubmit{Text: "conf"})
	p.Do(ActionMoveNext{})

	effects := p.Do(ActionBeginFilter{})
	if !reflect.DeepEqual(effects, []Effect{EffectBeginInput{Text: "conf"}}) {
		t.Errorf("BeginFilter effects = %#v, want an input holding %q", effects, "conf")
	}

	// Submitting the term untouched leaves the selection where it was.
	p.Do(ActionFilterSubmit{Text: "conf"})

	if got := selected(p.Frame()); got != "config.local.json" {
		t.Errorf("selected %q, want config.local.json", got)
	}
}

func TestFilterCancelPutsBackTheTermAndTheSelection(t *testing.T) {
	t.Parallel()

	p := newPicker(t, listing("app.json", "config.json", "config.local.json", "package.json"), "dir")
	p.Do(ActionBeginFilter{})
	p.Do(ActionFilterSubmit{Text: "conf"})
	p.Do(ActionMoveNext{})

	p.Do(ActionBeginFilter{})
	p.Do(ActionFilterChange{Text: "pack"})
	p.Do(ActionFilterCancel{})

	if p.Mode() != ModeNormal {
		t.Errorf("Mode = %d, want ModeNormal", p.Mode())
	}

	f := p.Frame()
	if f.Filter != "conf" || len(f.Names) != 2 {
		t.Errorf("Filter, Names = %q, %q, want the two conf files", f.Filter, f.Names)
	}

	if got := selected(f); got != "config.local.json" {
		t.Errorf("selected %q, want config.local.json", got)
	}
}

func TestClearFilterBringsEveryNameBack(t *testing.T) {
	t.Parallel()

	p := newPicker(t, listing("app.json", "config.json", "config.local.json"), "dir")
	p.Do(ActionBeginFilter{})
	p.Do(ActionFilterSubmit{Text: "conf"})
	p.Do(ActionMoveNext{})
	p.Do(ActionClearFilter{})

	f := p.Frame()
	if f.Filter != "" || len(f.Names) != 3 {
		t.Errorf("Filter, Names = %q, %q, want every name", f.Filter, f.Names)
	}

	if got := selected(f); got != "config.local.json" {
		t.Errorf("selected %q, want the name selected before", got)
	}
}

// Opening the selection.

func TestOpenAsksForTheSelectedFileInTheDirectory(t *testing.T) {
	t.Parallel()

	p := newPicker(t, listing("a.json", "b.json"), "logs")
	p.Do(ActionMoveNext{})

	want := []Effect{EffectOpen{Path: filepath.Join("logs", "b.json")}}
	if got := p.Do(ActionOpen{}); !reflect.DeepEqual(got, want) {
		t.Errorf("Open effects = %#v, want %#v", got, want)
	}
}

func TestOpenWithNoMatchesAsksForNothing(t *testing.T) {
	t.Parallel()

	p := newPicker(t, listing("a.json", "b.json"), "dir")
	p.Do(ActionBeginFilter{})
	p.Do(ActionFilterSubmit{Text: "zzz"})

	f := p.Frame()
	if len(f.Names) != 0 || f.Cursor != -1 {
		t.Fatalf("Names, Cursor = %q, %d, want no matches", f.Names, f.Cursor)
	}

	if got := p.Do(ActionOpen{}); got != nil {
		t.Errorf("Open effects = %#v, want none", got)
	}
}

// A file that would not open.

func TestFailedShowsTheErrorAndListsAgain(t *testing.T) {
	t.Parallel()

	l := listing("a.json", "b.json")
	p := newPicker(t, l, "dir")

	p.Failed(fs.ErrNotExist)

	if err := p.Frame().Err; !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Err = %v, want %v", err, fs.ErrNotExist)
	}

	if !reflect.DeepEqual(l.dirs, []string{"dir", "dir"}) {
		t.Errorf("listed %q, want dir twice", l.dirs)
	}
}

func TestFailedKeepsTheSelectionOnTheSameNameOrPlace(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		before []string
		after  []string
		moves  int
		want   string
	}{
		"the name is still there": {
			before: []string{"a.json", "b.json", "c.json"},
			after:  []string{"0.json", "a.json", "b.json", "c.json"},
			moves:  1,
			want:   "b.json",
		},
		"the name has gone": {
			before: []string{"a.json", "b.json", "c.json"},
			after:  []string{"a.json", "c.json"},
			moves:  1,
			want:   "c.json",
		},
		"the place is past the end": {
			before: []string{"a.json", "b.json", "c.json"},
			after:  []string{"a.json", "b.json"},
			moves:  2,
			want:   "b.json",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			l := &fakeDirLister{results: []listResult{
				{entries: files(tc.before...)},
				{entries: files(tc.after...)},
			}}
			p := newPicker(t, l, "dir")

			for range tc.moves {
				p.Do(ActionMoveNext{})
			}

			p.Failed(fs.ErrNotExist)

			if got := selected(p.Frame()); got != tc.want {
				t.Errorf("selected %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFailedKeepsTheFilter(t *testing.T) {
	t.Parallel()

	l := &fakeDirLister{results: []listResult{
		{entries: files("app.json", "config.json", "config.local.json")},
		{entries: files("app.json", "config.local.json", "config.new.json")},
	}}
	p := newPicker(t, l, "dir")
	p.Do(ActionBeginFilter{})
	p.Do(ActionFilterSubmit{Text: "conf"})

	p.Failed(fs.ErrNotExist)

	f := p.Frame()
	if f.Filter != "conf" {
		t.Errorf("Filter = %q, want %q", f.Filter, "conf")
	}

	want := []string{"config.local.json", "config.new.json"}
	if !reflect.DeepEqual(f.Names, want) {
		t.Errorf("Names = %q, want %q", f.Names, want)
	}
}

func TestFailedLeavesNothingWhenTheDirectoryHasNoCandidatesLeft(t *testing.T) {
	t.Parallel()

	tests := map[string]listResult{
		"no JSON file left":        {entries: files("notes.txt")},
		"directory cannot be read": {err: fs.ErrPermission},
	}

	for name, relisted := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			l := &fakeDirLister{results: []listResult{{entries: files("a.json")}, relisted}}
			p := newPicker(t, l, "dir")

			opening := errors.New("cannot parse")
			p.Failed(opening)

			f := p.Frame()
			if f.Total != 0 || len(f.Names) != 0 || f.Cursor != -1 {
				t.Errorf("Total, Names, Cursor = %d, %q, %d, want nothing to pick",
					f.Total, f.Names, f.Cursor)
			}

			// The failure shown is the one the reader caused, not the
			// listing's.
			if !errors.Is(f.Err, opening) {
				t.Errorf("Err = %v, want %v", f.Err, opening)
			}

			if p.Mode() != ModeNormal {
				t.Errorf("Mode = %d, want ModeNormal", p.Mode())
			}
		})
	}
}

func TestTheErrorLastsUntilTheNextAction(t *testing.T) {
	t.Parallel()

	p := newPicker(t, listing("a.json", "b.json"), "dir")
	p.Failed(fs.ErrNotExist)

	// A resize is not something the reader did to the picker.
	p.SetSize(5)

	if p.Frame().Err == nil {
		t.Fatal("Err cleared by a resize, want it kept")
	}

	p.Do(ActionMoveNext{})

	if err := p.Frame().Err; err != nil {
		t.Errorf("Err = %v after an action, want nil", err)
	}
}

// Help and leaving.

func TestHelpTakesNoOtherAction(t *testing.T) {
	t.Parallel()

	p := newPicker(t, listing("a.json", "b.json"), "dir")
	p.Do(ActionShowHelp{})

	if p.Mode() != ModeHelp {
		t.Fatalf("Mode = %d, want ModeHelp", p.Mode())
	}

	p.Do(ActionMoveNext{})

	if got := p.Do(ActionOpen{}); got != nil {
		t.Errorf("Open effects in help = %#v, want none", got)
	}

	p.Do(ActionCloseHelp{})

	if p.Mode() != ModeNormal {
		t.Errorf("Mode = %d, want ModeNormal", p.Mode())
	}

	if got := p.Frame().Cursor; got != 0 {
		t.Errorf("Cursor = %d, want the move made in help ignored", got)
	}
}

func TestQuitIsHonouredInEveryMode(t *testing.T) {
	t.Parallel()

	tests := map[string]Action{
		"normal": nil,
		"filter": ActionBeginFilter{},
		"help":   ActionShowHelp{},
	}

	for name, enter := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := newPicker(t, listing("a.json"), "dir")
			if enter != nil {
				p.Do(enter)
			}

			if got := p.Do(ActionQuit{}); !reflect.DeepEqual(got, []Effect{EffectQuit{}}) {
				t.Errorf("Quit effects = %#v, want EffectQuit", got)
			}
		})
	}
}
