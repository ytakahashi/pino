package application

import (
	"slices"
	"testing"

	"github.com/ytakahashi/pino/internal/application/documentview"
)

func TestSearchOpensATextPromptWithoutMovingTheCursor(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	standOn(t, app, "/server")

	effect := beginInput(t, app.Do(ActionSearch{}))
	if effect != (EffectBeginInput{}) {
		t.Errorf("ActionSearch effect = %+v, want an empty single-line input", effect)
	}
	if app.Mode() != ModeSearch {
		t.Errorf("Mode() = %v, want %v", app.Mode(), ModeSearch)
	}
	if prompt := app.Prompt(); prompt.Kind != PromptText || prompt.Title != "/" || prompt.Multiline {
		t.Errorf("Prompt() = %+v, want a single-line search prompt", prompt)
	}

	app.Do(ActionPromptChange{Text: "absent"})
	if got := app.Prompt().Error; got != noMatches {
		t.Errorf("Prompt().Error = %q, want %q", got, noMatches)
	}
	if got := cursorOf(app); got != "/server" {
		t.Errorf("typing a search moved the cursor to %q", got)
	}

	app.Do(ActionPromptChange{Text: "host"})
	if got := app.Prompt().Error; got != "" {
		t.Errorf("Prompt().Error = %q after a matching term, want empty", got)
	}

	app.Do(ActionShowHelp{})
	if app.Mode() != ModeSearch {
		t.Errorf("help interrupted the search; mode = %v", app.Mode())
	}
}

func TestSearchSubmitMovesToTheFirstMatchAtOrAfterTheCursor(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cursor string
		term   string
		want   string
	}{
		"a later match":         {cursor: "", term: "8443", want: "/server/ports/1"},
		"the cursor itself":     {cursor: "/server/host", term: "host", want: "/server/host"},
		"wrap to the beginning": {cursor: "/debug", term: "pino", want: "/name"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := session(t, sample(t))
			standOn(t, app, tc.cursor)
			acceptSearch(t, app, tc.term)

			if got := cursorOf(app); got != tc.want {
				t.Errorf("search selected %q, want %q", got, tc.want)
			}
			if app.Mode() != ModeNormal {
				t.Errorf("Mode() = %v after accepting a search, want %v", app.Mode(), ModeNormal)
			}
		})
	}
}

func TestSearchRefusalAndCancellationLeaveTheAcceptedSearchAlone(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	acceptSearch(t, app, "host")

	app.Do(ActionSearch{})
	app.Do(ActionPromptSubmit{Text: "absent"})
	if app.Mode() != ModeSearch || app.Prompt().Error != noMatches {
		t.Errorf("a missing term left mode=%v error=%q, want search and %q",
			app.Mode(), app.Prompt().Error, noMatches)
	}
	if got := app.Status().Search; got == nil || got.Query != "host" {
		t.Errorf("the accepted search became %+v, want host", got)
	}

	app.Do(ActionCancel{})
	if app.Mode() != ModeNormal {
		t.Errorf("Mode() = %v after cancelling, want %v", app.Mode(), ModeNormal)
	}
	if got := app.Status().Search; got == nil || got.Query != "host" {
		t.Errorf("cancelling left search %+v, want host", got)
	}
}

func TestEmptySearchSubmitClearsTheAcceptedSearch(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	acceptSearch(t, app, "host")

	app.Do(ActionSearch{})
	app.Do(ActionPromptSubmit{})

	if app.Mode() != ModeNormal || app.Status().Search != nil {
		t.Errorf("empty submit left mode=%v search=%+v, want normal and no search",
			app.Mode(), app.Status().Search)
	}
	if len(app.Frame().Matches) != 0 {
		t.Errorf("empty submit left matched rows %v", app.Frame().Matches)
	}
}

func TestSearchNextAndPrevWalkFromTheCurrentCursor(t *testing.T) {
	t.Parallel()

	app := session(t, searchWalkDocument(t))
	acceptSearch(t, app, "hit")

	if got := cursorOf(app); got != "/a" {
		t.Fatalf("first search selected %q, want /a", got)
	}

	app.Do(ActionMoveNext{}) // /b, which is not a match
	app.Do(ActionSearchNext{})
	if got := cursorOf(app); got != "/c" {
		t.Errorf("next from /b selected %q, want /c", got)
	}

	app.Do(ActionSearchPrev{})
	if got := cursorOf(app); got != "/a" {
		t.Errorf("previous selected %q, want /a", got)
	}

	app.Do(ActionSearchPrev{})
	if got := cursorOf(app); got != "/e" {
		t.Errorf("previous from the first match selected %q, want /e", got)
	}

	app.Do(ActionSearchNext{})
	if got := cursorOf(app); got != "/a" {
		t.Errorf("next from the last match selected %q, want /a", got)
	}
}

func TestSearchActionsDoNothingWithoutASearchOrADocument(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	before := stand(app)
	app.Do(ActionSearchNext{})
	app.Do(ActionSearchPrev{})
	if got := stand(app); got != before {
		t.Errorf("search navigation changed a session without a search:\n got %+v\nwant %+v", got, before)
	}

	empty := New(Deps{
		JSONView: documentview.NewJSONRenderer(),
		TreeView: documentview.NewTreeRenderer(),
	}, Config{})
	if effects := empty.Do(ActionSearch{}); effects != nil {
		t.Errorf("search without a document returned effects %v", effects)
	}
	if empty.Mode() != ModeNormal {
		t.Errorf("search without a document entered mode %v", empty.Mode())
	}
}

func TestSearchActionsDoNothingWhileAnotherFlowIsInProgress(t *testing.T) {
	t.Parallel()

	app := session(t, searchWalkDocument(t))
	acceptSearch(t, app, "hit")
	app.Do(ActionMoveNext{}) // Stand on the non-matching /b node.
	beginInput(t, app.Do(ActionEdit{}))

	flow := app.flow
	if effects := app.Do(ActionSearch{}); effects != nil {
		t.Errorf("search during an edit returned effects %v", effects)
	}
	app.Do(ActionSearchNext{})
	app.Do(ActionSearchPrev{})

	if app.flow != flow || app.Mode() != ModeEdit {
		t.Errorf("search actions replaced the edit flow with mode %v", app.Mode())
	}
	if got := cursorOf(app); got != "/b" {
		t.Errorf("search navigation during an edit selected %q, want /b", got)
	}
}

func TestSearchNavigationRevealsAncestors(t *testing.T) {
	t.Parallel()

	root := object(t,
		member("server", object(t,
			member("cache", object(t, member("needle", text(t, "found")))),
		)),
	)

	t.Run("a hidden descendant", func(t *testing.T) {
		t.Parallel()

		app := session(t, root)
		app.view.Collapse(pointer(t, "/server"))
		app.view.Collapse(pointer(t, "/server/cache"))
		app.settle(app.render())

		acceptSearch(t, app, "needle")

		if got := cursorOf(app); got != "/server/cache/needle" {
			t.Errorf("search selected %q, want /server/cache/needle", got)
		}
		if app.view.IsCollapsed(pointer(t, "/server")) || app.view.IsCollapsed(pointer(t, "/server/cache")) {
			t.Errorf("search left an ancestor folded: %v", foldsOf(app.view))
		}
	})

	t.Run("a matching folded container", func(t *testing.T) {
		t.Parallel()

		app := session(t, root)
		app.view.Collapse(pointer(t, "/server"))
		app.view.Collapse(pointer(t, "/server/cache"))
		app.settle(app.render())

		acceptSearch(t, app, "cache")

		if got := cursorOf(app); got != "/server/cache" {
			t.Errorf("search selected %q, want /server/cache", got)
		}
		if app.view.IsCollapsed(pointer(t, "/server")) {
			t.Error("search left the target's parent folded")
		}
		if !app.view.IsCollapsed(pointer(t, "/server/cache")) {
			t.Error("search unfolded the target container itself")
		}
	})
}

func TestSearchResultsRefreshAfterEditUndoAndRedo(t *testing.T) {
	t.Parallel()

	app := session(t, object(t, member("a", text(t, "hit"))))
	acceptSearch(t, app, "hit")

	app.Do(ActionEdit{})
	answer(app, "miss")
	assertSearchStatus(t, app, "hit", 0, 0)

	app.Do(ActionUndo{})
	assertSearchStatus(t, app, "hit", 1, 1)

	app.Do(ActionRedo{})
	assertSearchStatus(t, app, "hit", 0, 0)

	standOn(t, app, "")
	beginInput(t, app.Do(ActionAddChild{}))
	answer(app, "b")
	beginInput(t, app.Do(ActionPromptChoose{Key: 's'}))
	answer(app, "hit")
	assertSearchStatus(t, app, "hit", 1, 1)

	standOn(t, app, "")
	app.Do(ActionSearchNext{})
	if got := cursorOf(app); got != "/b" {
		t.Errorf("next after insertion selected %q, want /b", got)
	}
}

func TestReloadKeepsTheSearchAndOpenClearsIt(t *testing.T) {
	t.Parallel()

	parser := &fakeParser{root: object(t, member("a", text(t, "hit")))}
	files := &fakeFileStore{data: map[string][]byte{
		"a.json": []byte(testSource),
		"b.json": []byte(testSource),
	}}
	app := New(Deps{
		Parser:   parser,
		Files:    files,
		JSONView: documentview.NewJSONRenderer(),
		TreeView: documentview.NewTreeRenderer(),
	}, Config{})
	if err := app.Open("a.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	acceptSearch(t, app, "hit")

	parser.root = object(t, member("a", text(t, "miss")), member("b", text(t, "hit")))
	app.reload()
	assertSearchStatus(t, app, "hit", 0, 1)

	if err := app.Open("b.json"); err != nil {
		t.Fatalf("Open another document: %v", err)
	}
	if app.Status().Search != nil {
		t.Errorf("opening another document kept search %+v", app.Status().Search)
	}
}

func TestFrameMarksVisibleMatchesAndFoldedAncestorsInBothViews(t *testing.T) {
	t.Parallel()

	root := object(t,
		member("server", object(t,
			member("a", text(t, "hit")),
			member("b", text(t, "hit")),
		)),
		member("top", text(t, "hit")),
	)
	app := session(t, root)
	acceptSearch(t, app, "hit")
	app.view.Collapse(pointer(t, "/server"))
	app.settle(app.render())

	for _, view := range []ViewMode{ViewJSON, ViewTree} {
		app.view.ViewMode = view
		app.settle(app.render())

		if got, want := matchedPointers(app.Frame()), []string{"/server", "/top"}; !slices.Equal(got, want) {
			t.Errorf("%v matched rows point at %v, want %v", view, got, want)
		}
	}

	assertSearchStatus(t, app, "hit", 0, 3)
}

func TestSearchStatusFollowsTheCursor(t *testing.T) {
	t.Parallel()

	app := session(t, searchWalkDocument(t))
	acceptSearch(t, app, "hit")
	assertSearchStatus(t, app, "hit", 1, 3)

	app.Do(ActionMoveNext{})
	assertSearchStatus(t, app, "hit", 0, 3)

	app.Do(ActionSearchNext{})
	assertSearchStatus(t, app, "hit", 2, 3)
}

func TestFrameMarksAContainerOpeningRowRatherThanItsClosingRow(t *testing.T) {
	t.Parallel()

	app := session(t, sample(t))
	acceptSearch(t, app, "server")

	frame := app.Frame()
	if len(frame.Matches) != 1 {
		t.Fatalf("Frame().Matches = %v, want one row", frame.Matches)
	}

	line := frame.Lines[frame.Matches[0]]
	if line.Path.String() != "/server" || line.Kind != documentview.LineOpen {
		t.Errorf("matched line = path %q kind %v, want /server open", line.Path, line.Kind)
	}
}
