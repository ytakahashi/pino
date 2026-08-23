package application

import (
	"slices"
	"testing"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

func TestQueryUsesSmartcase(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		query string
		text  string
		want  bool
	}{
		"lowercase ignores case":       {query: "port", text: "ServerPort", want: true},
		"uppercase preserves case":     {query: "Port", text: "ServerPort", want: true},
		"uppercase refuses other case": {query: "Port", text: "serverport", want: false},
		"a substring is enough":        {query: "serve", text: "server", want: true},
		"Unicode letters are folded":   {query: "ä", text: "Ärger", want: true},
		"empty matches nothing":        {query: "", text: "anything", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := newQuery(tc.query).match(tc.text); got != tc.want {
				t.Errorf("newQuery(%q).match(%q) = %t, want %t", tc.query, tc.text, got, tc.want)
			}
		})
	}
}

func TestQueryKnowsWhenItIsEmpty(t *testing.T) {
	t.Parallel()

	if !newQuery("").isZero() {
		t.Error("newQuery(\"\").isZero() = false, want true")
	}

	if newQuery("x").isZero() {
		t.Error("newQuery(\"x\").isZero() = true, want false")
	}
}

func TestHitsSearchesNodeText(t *testing.T) {
	t.Parallel()

	root := object(t,
		member("Server", object(t,
			member("port", domain.NewNumber("1.50")),
			member("message", text(t, "say \"hi\"\nnext")),
		)),
		member("enabled", domain.NewBool(true)),
		member("disabled", domain.NewBool(false)),
		member("nothing", domain.NewNull()),
		member("items", domain.NewArray([]domain.Node{text(t, "alpha")})),
		member(`quoted"key`, text(t, "plain")),
		member("same", text(t, "same")),
	)

	tests := map[string]struct {
		root  domain.Node
		query string
		want  []string
	}{
		"object key":         {root: root, query: "server", want: []string{"/Server"}},
		"container key":      {root: root, query: "items", want: []string{"/items"}},
		"string value":       {root: root, query: `hi"`, want: []string{"/Server/message"}},
		"number spelling":    {root: root, query: ".50", want: []string{"/Server/port"}},
		"true value":         {root: root, query: "true", want: []string{"/enabled"}},
		"false value":        {root: root, query: "false", want: []string{"/disabled"}},
		"null value":         {root: root, query: "null", want: []string{"/nothing"}},
		"array value":        {root: root, query: "alpha", want: []string{"/items/0"}},
		"unescaped key":      {root: root, query: `quoted"`, want: []string{`/quoted"key`}},
		"key and value once": {root: root, query: "same", want: []string{"/same"}},
		"array index excluded": {
			root:  domain.NewArray([]domain.Node{text(t, "alpha")}),
			query: "0",
		},
		"quotes excluded": {
			root:  root,
			query: `\"`,
		},
		"escapes excluded": {
			root:  root,
			query: `\n`,
		},
		"root scalar": {
			root:  text(t, "root value"),
			query: "value",
			want:  []string{""},
		},
		"empty query": {root: root},
		"no document": {query: "anything"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := searchPointers(hits(tc.root, newQuery(tc.query))); !slices.Equal(got, tc.want) {
				t.Errorf("hits(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestHitsFollowExpandedDrawingOrder(t *testing.T) {
	t.Parallel()

	renderers := map[string]documentview.Renderer{
		"JSON": documentview.NewJSONRenderer(),
		"tree": documentview.NewTreeRenderer(),
	}
	queries := []string{"a", "1", "server", "t", "設定"}

	for documentName, root := range documents(t) {
		for rendererName, renderer := range renderers {
			for _, term := range queries {
				t.Run(documentName+"/"+rendererName+"/"+term, func(t *testing.T) {
					t.Parallel()

					got := searchPointers(hits(root, newQuery(term)))
					set := make(map[string]struct{}, len(got))
					for _, pointer := range got {
						set[pointer] = struct{}{}
					}

					var want []string
					for _, line := range renderer.Render(root, documentview.Options{}) {
						if line.Kind == documentview.LineClose {
							continue
						}

						if _, ok := set[line.Path.String()]; ok {
							want = append(want, line.Path.String())
						}
					}

					if !slices.Equal(got, want) {
						t.Errorf("hits(%q) = %v, drawing order gives %v", term, got, want)
					}
				})
			}
		}
	}
}

func TestSearchStateFindsTheHitsAroundTheCursor(t *testing.T) {
	t.Parallel()

	root := object(t,
		member("a", text(t, "hit")),
		member("b", text(t, "miss")),
		member("c", text(t, "hit")),
		member("d", text(t, "miss")),
		member("e", text(t, "hit")),
		member("f", text(t, "miss")),
	)

	tests := map[string]struct {
		cursor     domain.Path
		wantPassed int
		wantOn     bool
		wantNext   string
		wantPrev   string
	}{
		"before the first hit": {
			cursor: domain.Path{}, wantNext: "/a", wantPrev: "/e",
		},
		"on the first hit": {
			cursor: path(domain.KeySegment("a")), wantOn: true, wantNext: "/c", wantPrev: "/e",
		},
		"between hits": {
			cursor: path(domain.KeySegment("b")), wantPassed: 1, wantNext: "/c", wantPrev: "/a",
		},
		"on the middle hit": {
			cursor: path(domain.KeySegment("c")), wantPassed: 1, wantOn: true, wantNext: "/e", wantPrev: "/a",
		},
		"on the last hit": {
			cursor: path(domain.KeySegment("e")), wantPassed: 2, wantOn: true, wantNext: "/a", wantPrev: "/c",
		},
		"after the last hit": {
			cursor: path(domain.KeySegment("f")), wantPassed: 3, wantNext: "/a", wantPrev: "/e",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			state := searchState{query: newQuery("hit")}
			state.refresh(root, tc.cursor)

			if got := searchPointers(state.hits); !slices.Equal(got, []string{"/a", "/c", "/e"}) {
				t.Errorf("hits = %v, want [/a /c /e]", got)
			}
			if state.passed != tc.wantPassed {
				t.Errorf("passed = %d, want %d", state.passed, tc.wantPassed)
			}
			if state.on != tc.wantOn {
				t.Errorf("on = %t, want %t", state.on, tc.wantOn)
			}

			assertSearchDestination(t, "next", state.next, tc.wantNext)
			assertSearchDestination(t, "prev", state.prev, tc.wantPrev)
		})
	}
}

func TestSearchStateReturnsNoDestinationWithoutHits(t *testing.T) {
	t.Parallel()

	state := searchState{query: newQuery("absent")}
	state.refresh(sample(t), domain.Path{})

	if _, ok := state.next(); ok {
		t.Error("next() found a destination without hits")
	}
	if _, ok := state.prev(); ok {
		t.Error("prev() found a destination without hits")
	}
}

func TestSearchStatePlacesDescendantHitsAfterAContainerCursor(t *testing.T) {
	t.Parallel()

	root := object(t,
		member("server", object(t, member("port", text(t, "hit")))),
		member("z", text(t, "hit")),
	)
	state := searchState{query: newQuery("hit")}
	state.refresh(root, path(domain.KeySegment("server")))

	if state.passed != 0 || state.on {
		t.Errorf("passed = %d, on = %t; want 0, false", state.passed, state.on)
	}
	assertSearchDestination(t, "next", state.next, "/server/port")
	assertSearchDestination(t, "prev", state.prev, "/z")
}

func TestSearchStateTreatsAMissingCursorAsFollowingEveryHit(t *testing.T) {
	t.Parallel()

	root := object(t,
		member("server", object(t, member("port", text(t, "hit")))),
		member("z", text(t, "hit")),
	)
	state := searchState{query: newQuery("hit")}
	state.refresh(root, path(domain.KeySegment("gone")))

	if state.passed != 2 || state.on {
		t.Errorf("passed = %d, on = %t; want 2, false", state.passed, state.on)
	}
	assertSearchDestination(t, "next", state.next, "/server/port")
	assertSearchDestination(t, "prev", state.prev, "/z")
}

func TestSearchStateWrapsASingleHitToItself(t *testing.T) {
	t.Parallel()

	root := object(t, member("a", text(t, "hit")))
	state := searchState{query: newQuery("hit")}
	state.refresh(root, path(domain.KeySegment("a")))

	assertSearchDestination(t, "next", state.next, "/a")
	assertSearchDestination(t, "prev", state.prev, "/a")
}

func TestSearchStateReplacesDerivedValuesOnRefresh(t *testing.T) {
	t.Parallel()

	root := object(t, member("a", text(t, "hit")))
	state := searchState{query: newQuery("hit")}
	state.refresh(root, path(domain.KeySegment("a")))

	tests := map[string]struct {
		query query
		root  domain.Node
	}{
		"a term without matches": {query: newQuery("absent"), root: root},
		"an empty term":          {query: newQuery(""), root: root},
		"no document":            {query: newQuery("hit")},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			snapshot := state
			snapshot.query = tc.query
			snapshot.refresh(tc.root, path(domain.KeySegment("a")))

			if len(snapshot.hits) != 0 || snapshot.passed != 0 || snapshot.on {
				t.Errorf("refresh left hits=%v, passed=%d, on=%t",
					searchPointers(snapshot.hits), snapshot.passed, snapshot.on)
			}
		})
	}
}

func searchPointers(paths []domain.Path) []string {
	pointers := make([]string, len(paths))
	for i, p := range paths {
		pointers[i] = p.String()
	}

	return pointers
}

func assertSearchDestination(
	t *testing.T,
	name string,
	destination func() (domain.Path, bool),
	want string,
) {
	t.Helper()

	got, ok := destination()
	if !ok {
		t.Fatalf("%s() found no destination, want %q", name, want)
	}

	if got.String() != want {
		t.Errorf("%s() = %q, want %q", name, got, want)
	}
}
