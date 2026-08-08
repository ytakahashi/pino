package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// path builds a Path the way the renderer walks a tree.
func path(segs ...domain.Segment) domain.Path {
	p := domain.Path{}
	for _, s := range segs {
		p = p.Child(s)
	}

	return p
}

// rows renders a document the way the session does, so that the fixtures obey
// what the walking functions rely on: an open row for every close row, and a
// depth that grows by one per level. Hand-written rows can say things a
// renderer never would.
func rows(t *testing.T, root domain.Node, folded map[string]struct{}) []Line {
	t.Helper()

	return NewJSONRenderer().Render(root, RenderOptions{Collapsed: folded})
}

// sample is a document with a container holding a container, an array of
// scalars, and members on either side of them.
//
//	 0  open    /                {
//	 1  single  /name            "name": "pino",
//	 2  open    /server          "server": {
//	 3  single  /server/host       "host": "localhost",
//	 4  open    /server/ports      "ports": [
//	 5  single  /server/ports/0      8080,
//	 6  single  /server/ports/1      8443
//	 7  close   /server/ports      ],
//	 8  single  /server/tls        "tls": true
//	 9  close   /server          },
//	10  single  /debug           "debug": false
//	11  close   /                }
func sample(t *testing.T) domain.Node {
	t.Helper()

	return object(t,
		member("name", text(t, "pino")),
		member("server", object(t,
			member("host", text(t, "localhost")),
			member("ports", domain.NewArray([]domain.Node{
				domain.NewNumber("8080"),
				domain.NewNumber("8443"),
			})),
			member("tls", domain.NewBool(true)),
		)),
		member("debug", domain.NewBool(false)),
	)
}

func pointerAt(t *testing.T, lines []Line, row int) string {
	t.Helper()

	if row < 0 || row >= len(lines) {
		t.Fatalf("row %d is out of range for %d rows", row, len(lines))
	}

	return lines[row].Path.String()
}

func TestIndexOf(t *testing.T) {
	t.Parallel()

	lines := rows(t, sample(t), nil)

	tests := map[string]struct {
		cursor domain.Path
		want   int
	}{
		"the root":         {cursor: domain.Path{}, want: 0},
		"a member":         {cursor: path(domain.KeySegment("name")), want: 1},
		"a container":      {cursor: path(domain.KeySegment("server")), want: 2},
		"an array element": {cursor: path(domain.KeySegment("server"), domain.KeySegment("ports"), domain.IndexSegment(1)), want: 6},
		"not in the document": {
			cursor: path(domain.KeySegment("missing")),
			want:   -1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := indexOf(lines, tc.cursor); got != tc.want {
				t.Errorf("indexOf(%q) = %d, want %d", tc.cursor, got, tc.want)
			}
		})
	}
}

// The root row carries the path of the whole document, so a cursor on the root
// must find the opening row and not the closing one.
func TestIndexOfPrefersTheOpeningRow(t *testing.T) {
	t.Parallel()

	lines := rows(t, sample(t), nil)

	if got := indexOf(lines, domain.Path{}); got != 0 {
		t.Errorf("indexOf(root) = %d, want 0; the closing row shares its path", got)
	}
}

func TestVisibleRow(t *testing.T) {
	t.Parallel()

	deep := path(domain.KeySegment("server"), domain.KeySegment("ports"), domain.IndexSegment(1))

	tests := map[string]struct {
		folded map[string]struct{}
		cursor domain.Path
		want   string // the pointer of the row it settles on
	}{
		"on screen": {
			cursor: deep,
			want:   "/server/ports/1",
		},
		"one level folded away": {
			folded: folded("/server/ports"),
			cursor: deep,
			want:   "/server/ports",
		},
		"two levels folded away": {
			folded: folded("/server"),
			cursor: deep,
			want:   "/server",
		},
		"the whole document folded": {
			folded: folded(""),
			cursor: deep,
			want:   "",
		},
		"a node that was never there": {
			cursor: path(domain.KeySegment("missing"), domain.KeySegment("deeper")),
			want:   "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			lines := rows(t, sample(t), tc.folded)

			got := visibleRow(lines, tc.cursor)
			if got < 0 {
				t.Fatalf("visibleRow(%q) = -1, want the row for %q", tc.cursor, tc.want)
			}

			if pointer := pointerAt(t, lines, got); pointer != tc.want {
				t.Errorf("visibleRow(%q) settled on %q, want %q", tc.cursor, pointer, tc.want)
			}
		})
	}
}

func TestVisibleRowWithoutRows(t *testing.T) {
	t.Parallel()

	if got := visibleRow(nil, path(domain.KeySegment("a"))); got != -1 {
		t.Errorf("visibleRow(nil) = %d, want -1", got)
	}

	if got := visibleRow(nil, domain.Path{}); got != -1 {
		t.Errorf("visibleRow(nil, root) = %d, want -1", got)
	}
}

func TestFirstRow(t *testing.T) {
	t.Parallel()

	if got := firstRow(rows(t, sample(t), nil)); got != 0 {
		t.Errorf("firstRow() = %d, want 0", got)
	}

	if got := firstRow(nil); got != -1 {
		t.Errorf("firstRow(nil) = %d, want -1", got)
	}
}

// Moving down visits every node in document order and never stops on a closing
// row, which is what makes a flat list of rows behave like a walk of the tree.
func TestNextRowWalksTheWholeDocument(t *testing.T) {
	t.Parallel()

	lines := rows(t, sample(t), nil)

	want := []string{
		"", "/name", "/server", "/server/host", "/server/ports",
		"/server/ports/0", "/server/ports/1", "/server/tls", "/debug",
	}

	var got []string
	for row := 0; row >= 0; row = nextRow(lines, row) {
		got = append(got, pointerAt(t, lines, row))
	}

	if len(got) != len(want) {
		t.Fatalf("walked %d rows %v, want %d %v", len(got), got, len(want), want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d landed on %q, want %q", i, got[i], want[i])
		}
	}
}

// Walking back up retraces exactly the rows walking down visited. Moving down
// and then up by the same number of steps has to end where it started, which
// it would not if the two disagreed about which rows to skip.
func TestPrevRowRetracesNextRow(t *testing.T) {
	t.Parallel()

	lines := rows(t, sample(t), nil)

	var down []int
	for row := 0; row >= 0; row = nextRow(lines, row) {
		down = append(down, row)
	}

	var up []int
	for row := down[len(down)-1]; row >= 0; row = prevRow(lines, row) {
		up = append(up, row)
	}

	if len(up) != len(down) {
		t.Fatalf("walked down %d rows and back up %d", len(down), len(up))
	}

	for i, row := range up {
		want := down[len(down)-1-i]
		if row != want {
			t.Errorf("step %d up landed on %q, want %q",
				i, pointerAt(t, lines, row), pointerAt(t, lines, want))
		}
	}
}

// Moving up from the member below a container lands on that container's last
// descendant rather than on the container itself: the closing rows between
// them are skipped, and what is above them is the deepest row, not the shallow
// one that opened it.
func TestPrevRowEntersTheContainerAbove(t *testing.T) {
	t.Parallel()

	lines := rows(t, sample(t), nil)

	from := indexOf(lines, path(domain.KeySegment("debug")))
	if got := pointerAt(t, lines, prevRow(lines, from)); got != "/server/tls" {
		t.Errorf("prevRow from /debug landed on %q, want /server/tls", got)
	}

	from = indexOf(lines, path(domain.KeySegment("server"), domain.KeySegment("tls")))
	if got := pointerAt(t, lines, prevRow(lines, from)); got != "/server/ports/1" {
		t.Errorf("prevRow from /server/tls landed on %q, want /server/ports/1", got)
	}
}

func TestNextAndPrevRowStopAtTheEnds(t *testing.T) {
	t.Parallel()

	lines := rows(t, sample(t), nil)

	if got := prevRow(lines, 0); got != -1 {
		t.Errorf("prevRow(0) = %d, want -1", got)
	}

	// Every row below the last node is a closing one, so there is nowhere to go.
	last := indexOf(lines, path(domain.KeySegment("debug")))
	if got := nextRow(lines, last); got != -1 {
		t.Errorf("nextRow(last node) = %d, want -1", got)
	}
}

func TestParentRow(t *testing.T) {
	t.Parallel()

	lines := rows(t, sample(t), nil)

	tests := map[string]struct {
		cursor domain.Path
		want   string
	}{
		"a member of the root": {
			cursor: path(domain.KeySegment("name")),
			want:   "",
		},
		"a member of a nested object": {
			cursor: path(domain.KeySegment("server"), domain.KeySegment("host")),
			want:   "/server",
		},
		"an array element": {
			cursor: path(domain.KeySegment("server"), domain.KeySegment("ports"), domain.IndexSegment(0)),
			want:   "/server/ports",
		},
		// /debug follows the closing rows of /server/ports and /server, both
		// of which a search by depth alone could stop on.
		"after a sibling container": {
			cursor: path(domain.KeySegment("debug")),
			want:   "",
		},
		// /server/tls follows the closing row of the array beside it.
		"after a sibling array": {
			cursor: path(domain.KeySegment("server"), domain.KeySegment("tls")),
			want:   "/server",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := parentRow(lines, indexOf(lines, tc.cursor))
			if got < 0 {
				t.Fatalf("parentRow(%q) = -1, want the row for %q", tc.cursor, tc.want)
			}

			if pointer := pointerAt(t, lines, got); pointer != tc.want {
				t.Errorf("parentRow(%q) = %q, want %q", tc.cursor, pointer, tc.want)
			}
		})
	}
}

func TestParentRowOfTheRoot(t *testing.T) {
	t.Parallel()

	lines := rows(t, sample(t), nil)

	if got := parentRow(lines, 0); got != -1 {
		t.Errorf("parentRow(root) = %d, want -1", got)
	}

	if got := parentRow(lines, -1); got != -1 {
		t.Errorf("parentRow(-1) = %d, want -1", got)
	}
}

func TestFirstChildRow(t *testing.T) {
	t.Parallel()

	lines := rows(t, sample(t), nil)

	server := indexOf(lines, path(domain.KeySegment("server")))
	if got := firstChildRow(lines, server); pointerAt(t, lines, got) != "/server/host" {
		t.Errorf("firstChildRow(/server) = %q, want /server/host", pointerAt(t, lines, got))
	}

	if got := firstChildRow(lines, indexOf(lines, path(domain.KeySegment("debug")))); got != -1 {
		t.Errorf("firstChildRow(a scalar) = %d, want -1", got)
	}

	if got := firstChildRow(lines, len(lines)-1); got != -1 {
		t.Errorf("firstChildRow(a closing row) = %d, want -1", got)
	}

	if got := firstChildRow(lines, -1); got != -1 {
		t.Errorf("firstChildRow(-1) = %d, want -1", got)
	}
}

// A folded container has no children on screen, so moving in has to unfold it
// first rather than stepping onto whatever row follows.
func TestFirstChildRowOfAFoldedContainer(t *testing.T) {
	t.Parallel()

	lines := rows(t, sample(t), folded("/server"))

	server := indexOf(lines, path(domain.KeySegment("server")))
	if got := firstChildRow(lines, server); got != -1 {
		t.Errorf("firstChildRow(a folded container) = %d, want -1", got)
	}
}

// An empty container is drawn on one row and has nothing to step into either.
func TestFirstChildRowOfAnEmptyContainer(t *testing.T) {
	t.Parallel()

	lines := rows(t, object(t, member("opts", object(t))), nil)

	if got := firstChildRow(lines, indexOf(lines, path(domain.KeySegment("opts")))); got != -1 {
		t.Errorf("firstChildRow(an empty container) = %d, want -1", got)
	}
}

// The end of a document is its last node, not its last row: the rows closing
// everything still open come after it.
func TestLastRow(t *testing.T) {
	t.Parallel()

	lines := rows(t, sample(t), nil)

	if got := pointerAt(t, lines, lastRow(lines)); got != "/debug" {
		t.Errorf("lastRow() = %q, want /debug", got)
	}

	if got := lastRow(nil); got != -1 {
		t.Errorf("lastRow(nil) = %d, want -1", got)
	}

	// A document that is a single value has that value as its last node.
	scalar := rows(t, text(t, "only"), nil)
	if got := lastRow(scalar); got != 0 {
		t.Errorf("lastRow(a single value) = %d, want 0", got)
	}
}

func TestNearestRow(t *testing.T) {
	t.Parallel()

	lines := rows(t, sample(t), nil)
	closing := indexOf(lines, path(domain.KeySegment("server"), domain.KeySegment("ports"))) + 3

	if lines[closing].Kind != LineClose {
		t.Fatalf("the fixture changed: row %d is not a closing row", closing)
	}

	tests := map[string]struct {
		from, dir int
		want      string
	}{
		"already on a node":            {from: 1, dir: 1, want: "/name"},
		"on a closing row, going down": {from: closing, dir: 1, want: "/server/tls"},
		"on a closing row, going up":   {from: closing, dir: -1, want: "/server/ports/1"},

		// Past the end there is nothing below, so the search turns around.
		"past the end":   {from: len(lines) + 10, dir: 1, want: "/debug"},
		"past the start": {from: -10, dir: -1, want: ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := nearestRow(lines, tc.from, tc.dir)
			if got < 0 {
				t.Fatalf("nearestRow(%d, %d) = -1, want the row for %q", tc.from, tc.dir, tc.want)
			}

			if pointer := pointerAt(t, lines, got); pointer != tc.want {
				t.Errorf("nearestRow(%d, %d) = %q, want %q", tc.from, tc.dir, pointer, tc.want)
			}
		})
	}

	if got := nearestRow(nil, 0, 1); got != -1 {
		t.Errorf("nearestRow(nil) = %d, want -1", got)
	}
}

// Whatever it is asked, it answers a row the cursor can occupy. Jumping by a
// count of rows relies on that: it lands wherever the arithmetic puts it.
func TestNearestRowAlwaysLandsOnANode(t *testing.T) {
	t.Parallel()

	lines := rows(t, sample(t), nil)

	for from := -3; from < len(lines)+3; from++ {
		for _, dir := range []int{1, -1} {
			got := nearestRow(lines, from, dir)
			if got < 0 || got >= len(lines) {
				t.Fatalf("nearestRow(%d, %d) = %d, outside the document", from, dir, got)
			}

			if lines[got].Kind == LineClose {
				t.Fatalf("nearestRow(%d, %d) landed on a closing row", from, dir)
			}
		}
	}
}

func TestClampScroll(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		scroll, cursor, height, total int
		want                          int
	}{
		"the document fits":             {scroll: 0, cursor: 3, height: 10, total: 5, want: 0},
		"it fits after having scrolled": {scroll: 4, cursor: 3, height: 10, total: 5, want: 0},

		"the cursor is already on screen": {scroll: 5, cursor: 7, height: 10, total: 40, want: 5},
		"the cursor went above":           {scroll: 5, cursor: 2, height: 10, total: 40, want: 2},
		"the cursor went below":           {scroll: 5, cursor: 20, height: 10, total: 40, want: 11},
		"the cursor is on the last row":   {scroll: 0, cursor: 39, height: 10, total: 40, want: 30},
		"the cursor is on the first row":  {scroll: 20, cursor: 0, height: 10, total: 40, want: 0},

		// Folding rows away can leave the window past the end of a document
		// that is still longer than the screen.
		"the document shrank": {scroll: 30, cursor: -1, height: 10, total: 20, want: 10},

		// Showing the cursor wins over staying where the window was, even when
		// that means moving it further than the end of the document would.
		"shrank with the cursor near the top": {scroll: 30, cursor: 5, height: 10, total: 20, want: 5},

		// A window one row tall still follows the cursor.
		"a window of one row": {scroll: 0, cursor: 7, height: 1, total: 40, want: 7},

		// Nothing has said how big the screen is yet.
		"no window":        {scroll: 3, cursor: 7, height: 0, total: 40, want: 0},
		"negative height":  {scroll: 3, cursor: 7, height: -1, total: 40, want: 0},
		"nothing to draw":  {scroll: 3, cursor: -1, height: 10, total: 0, want: 0},
		"no cursor at all": {scroll: 4, cursor: -1, height: 10, total: 40, want: 4},

		// A scroll that arrived out of range is brought back either way.
		"a negative offset": {scroll: -5, cursor: -1, height: 10, total: 40, want: 0},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := clampScroll(tc.scroll, tc.cursor, tc.height, tc.total)
			if got != tc.want {
				t.Errorf("clampScroll(scroll=%d, cursor=%d, height=%d, total=%d) = %d, want %d",
					tc.scroll, tc.cursor, tc.height, tc.total, got, tc.want)
			}
		})
	}
}

// Whatever it is given, the answer has to be a window the cursor can be seen
// through. This is the property the rest of the layer relies on.
func TestClampScrollKeepsTheCursorInTheWindow(t *testing.T) {
	t.Parallel()

	const (
		height = 10
		total  = 40
	)

	for scroll := -5; scroll <= total+5; scroll += 5 {
		for cursor := range total {
			got := clampScroll(scroll, cursor, height, total)

			if got < 0 || got > total-height {
				t.Fatalf("clampScroll(%d, %d, ...) = %d, outside [0, %d]",
					scroll, cursor, got, total-height)
			}

			if cursor < got || cursor >= got+height {
				t.Fatalf("clampScroll(%d, %d, ...) = %d, which does not show the cursor",
					scroll, cursor, got)
			}
		}
	}
}
