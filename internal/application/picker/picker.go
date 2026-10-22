// Package picker is the list of files to choose a document from: which names
// are candidates, how they are narrowed and ordered, and where the selection
// and the window onto them are.
//
// It is a package of its own, beside rather than inside application, so that
// the picker and the session cannot reach into each other: the picker holds no
// document and returns a path, and whoever drives both is the only place that
// knows of the two. Like the session, it takes Actions and answers with
// Effects, so it is exercised without a terminal.
package picker

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/ytakahashi/pino/internal/application/textmatch"
)

// Mode is what the next key press means.
type Mode uint8

const (
	ModeNormal Mode = iota
	ModeFilter      // typing a filter term
	ModeHelp
)

// Frame is one drawable picture of the picker.
//
// Names are every candidate the filter lets through rather than the part that
// fits, as the session's Frame does with its rows: where the cut begins is
// Scroll, and making it is the business of whoever owns the terminal. Names
// are as the file system spells them; making them safe to print is left to
// that same layer, since opening one needs the name unchanged.
type Frame struct {
	Dir   string
	Names []string

	// Cursor indexes Names, and is -1 when there are none.
	Cursor int

	// Scroll is the first name to draw.
	Scroll int

	// Total is how many candidates there are before filtering.
	Total int

	// Filter is the term in effect, including one still being typed.
	Filter string

	// Err is the last failure to open a file. It lasts until the next Action.
	Err error
}

// Picker is the whole state of the list of files.
type Picker struct {
	lister DirLister

	// dir is where the candidates are listed from. It does not change yet, but
	// the list is always built from it, so moving between directories is a
	// matter of assigning it.
	dir string

	// all is every candidate in order, and shown the ones filter lets through.
	all    []string
	shown  []string
	filter textmatch.Query

	// before is what ActionFilterCancel puts back: the term in effect and the
	// name selected when typing began.
	before struct {
		filter textmatch.Query
		name   string
	}

	mode   Mode
	cursor int
	scroll int
	height int
	err    error
}

// New lists dir. A directory that cannot be read is an error, while one with
// no candidates is not: whether that ends the program is the caller's call,
// made by asking Empty.
func New(l DirLister, dir string) (*Picker, error) {
	entries, err := l.List(dir)
	if err != nil {
		return nil, err
	}

	p := &Picker{lister: l, dir: dir, all: candidates(entries)}
	p.narrow()
	p.place("", 0)

	return p, nil
}

// candidates is the names that can be picked, in order.
//
// This is where pino's rule lives: a file whose name ends in .json, in any
// case. The lister reports what the file system holds and has no say in it.
// Names are sorted here rather than trusted to arrive sorted, so that another
// order is a change to this function alone.
func candidates(entries []DirEntry) []string {
	var names []string

	for _, e := range entries {
		if !e.Dir && strings.EqualFold(filepath.Ext(e.Name), ".json") {
			names = append(names, e.Name)
		}
	}

	slices.Sort(names)

	return names
}

// Empty reports whether there is nothing to pick from, filtering aside.
func (p *Picker) Empty() bool { return len(p.all) == 0 }

// Mode is what the next key press means.
func (p *Picker) Mode() Mode { return p.mode }

// SetSize reports how many names the window can show at once.
//
// It is not an Action, because a resize is not something the reader did to
// the picker and must not clear a failure they have yet to read.
func (p *Picker) SetSize(height int) {
	p.height = max(height, 0)
	p.fit()
}

// Frame returns what there is to draw.
func (p *Picker) Frame() Frame {
	return Frame{
		Dir:    p.dir,
		Names:  slices.Clone(p.shown),
		Cursor: p.cursor,
		Scroll: p.scroll,
		Total:  len(p.all),
		Filter: p.filter.Text(),
		Err:    p.err,
	}
}

// Do applies an Action and returns the work the presentation layer has to
// carry out.
//
// Each Action is honoured only in the mode whose keys can send it; one that
// arrives in another mode is ignored rather than acted on out of context.
func (p *Picker) Do(act Action) []Effect {
	// A failure is shown until the reader does something next. Nothing is lost
	// by missing it: the only thing left to do about a file that would not
	// open is to pick another.
	p.err = nil

	if _, ok := act.(ActionQuit); ok {
		return []Effect{EffectQuit{}}
	}

	switch p.mode {
	case ModeNormal:
		return p.doNormal(act)

	case ModeFilter:
		p.doFilter(act)

	case ModeHelp:
		if _, ok := act.(ActionCloseHelp); ok {
			p.mode = ModeNormal
		}
	}

	return nil
}

func (p *Picker) doNormal(act Action) []Effect {
	switch act := act.(type) {
	case ActionMoveNext:
		p.moveTo(p.cursor + 1)

	case ActionMovePrev:
		p.moveTo(p.cursor - 1)

	case ActionMoveFirst:
		p.moveTo(0)

	case ActionMoveLast:
		p.moveTo(len(p.shown) - 1)

	case ActionScrollHalfDown:
		p.scrollHalf(+1)

	case ActionScrollHalfUp:
		p.scrollHalf(-1)

	case ActionScrollBy:
		p.scrollBy(act.Rows)

	case ActionBeginFilter:
		return p.beginFilter()

	case ActionClearFilter:
		// Every name the filter let through is still there afterwards, so the
		// selection stays on the one it was on rather than going to the top.
		name := p.selected()
		p.filter = textmatch.New("")
		p.narrow()
		p.place(name, 0)

	case ActionOpen:
		if p.cursor < 0 {
			return nil
		}

		return []Effect{EffectOpen{Path: filepath.Join(p.dir, p.shown[p.cursor])}}

	case ActionShowHelp:
		p.mode = ModeHelp
	}

	return nil
}

func (p *Picker) doFilter(act Action) {
	switch act := act.(type) {
	case ActionFilterChange:
		p.setFilter(act.Text)

	case ActionFilterSubmit:
		p.setFilter(act.Text)
		p.mode = ModeNormal

	case ActionFilterCancel:
		p.filter = p.before.filter
		p.narrow()
		p.place(p.before.name, 0)
		p.mode = ModeNormal
	}
}

// beginFilter starts typing a term, from the one already in effect.
//
// The box is seeded with that term rather than left empty. The list narrows as
// the term is typed, so an empty box would bring every candidate back the
// moment typing began, and what is typed next is usually an adjustment of the
// term rather than a new one.
func (p *Picker) beginFilter() []Effect {
	p.before.filter = p.filter
	p.before.name = p.selected()
	p.mode = ModeFilter

	return []Effect{EffectBeginInput{Text: p.filter.Text()}}
}

// setFilter makes text the term in effect and selects the first name it lets
// through.
//
// A report of the term already in effect changes nothing, so a box that
// reports on every key, or submitting a term untouched, leaves the selection
// where it was.
func (p *Picker) setFilter(text string) {
	if text == p.filter.Text() {
		return
	}

	// An empty term matches every name, which is what an empty filter means
	// here: unlike a search, where it means nothing to look for.
	p.filter = textmatch.New(text)
	p.narrow()
	p.place("", 0)
}

// Failed reports that the file last asked for could not be opened.
//
// The list is read again, since a file that has gone or been renamed should
// not stay on it. The term in effect is kept, and the selection stays on the
// same name if it is still there, or at the same place otherwise. err is what
// is shown even when listing fails too: it answers what the reader just did.
// A directory that can no longer be read leaves no candidates, as the names
// listed before cannot be trusted to open either.
func (p *Picker) Failed(err error) {
	name, pos := p.selected(), p.cursor

	entries, listErr := p.lister.List(p.dir)
	if listErr != nil {
		entries = nil
	}

	p.all = candidates(entries)
	p.narrow()
	p.place(name, pos)
	p.err = err
}

// selected is the name under the cursor, or "" when there is none.
func (p *Picker) selected() string {
	if p.cursor < 0 {
		return ""
	}

	return p.shown[p.cursor]
}

// narrow rebuilds shown from all under the term in effect.
func (p *Picker) narrow() {
	p.shown = nil

	for _, name := range p.all {
		if p.filter.Match(name) {
			p.shown = append(p.shown, name)
		}
	}
}

// place selects name if it is shown, and the name at pos otherwise, with pos
// brought within the list.
func (p *Picker) place(name string, pos int) {
	if i := slices.Index(p.shown, name); name != "" && i >= 0 {
		pos = i
	}

	p.moveTo(pos)
}

// moveTo selects the name at i, or the nearest end of the list.
func (p *Picker) moveTo(i int) {
	p.cursor = min(max(i, 0), len(p.shown)-1)
	p.fit()
}

// scrollHalf moves the window and the selection half a window, downwards for
// a positive dir. The window is moved here rather than left to follow the
// selection, so that the selection keeps its place on the screen.
func (p *Picker) scrollHalf(dir int) {
	if len(p.shown) == 0 {
		return
	}

	// Never less than a row: a window too small to halve still has to move.
	step := max(p.height/2, 1) * dir

	p.scroll = clampScroll(p.scroll+step, -1, p.height, len(p.shown))
	p.moveTo(p.cursor + step)
}

// scrollBy moves the window by rows, bringing the selection to the edge it
// went out by if it is left outside.
func (p *Picker) scrollBy(rows int) {
	if len(p.shown) == 0 || p.height <= 0 {
		return
	}

	p.scroll = clampScroll(p.scroll+rows, -1, p.height, len(p.shown))
	p.moveTo(min(max(p.cursor, p.scroll), p.scroll+p.height-1))
}

// fit puts the window where the selection is in it.
func (p *Picker) fit() {
	p.scroll = clampScroll(p.scroll, p.cursor, p.height, len(p.shown))
}

// clampScroll is the first name to draw so that cursor is in the window,
// moving as little as possible from scroll. A cursor of -1 asks only for
// scroll to be brought within range.
//
// It follows the rule the session scrolls a document by, without a margin.
func clampScroll(scroll, cursor, height, total int) int {
	if height <= 0 || total <= height {
		return 0
	}

	if cursor >= 0 {
		scroll = min(scroll, cursor)
		scroll = max(scroll, cursor-height+1)
	}

	return min(max(scroll, 0), total-height)
}
