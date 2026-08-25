package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

func acceptSearch(t *testing.T, app *App, term string) {
	t.Helper()

	beginInput(t, app.Do(ActionSearch{}))
	app.Do(ActionPromptChange{Text: term})
	if err := app.Prompt().Error; err != "" {
		t.Fatalf("search %q was refused: %s", term, err)
	}
	app.Do(ActionPromptSubmit{Text: term})
	if app.Mode() != ModeNormal {
		t.Fatalf("search %q left mode %v, want normal", term, app.Mode())
	}
}

func searchWalkDocument(t *testing.T) domain.Node {
	t.Helper()

	return object(t,
		member("a", text(t, "hit")),
		member("b", text(t, "miss")),
		member("c", text(t, "hit")),
		member("d", text(t, "miss")),
		member("e", text(t, "hit")),
	)
}

func assertSearchStatus(t *testing.T, app *App, query string, at, total int) {
	t.Helper()

	got := app.Status().Search
	if got == nil {
		t.Fatalf("Status().Search = nil, want %q %d/%d", query, at, total)
	}
	if got.Query != query || got.At != at || got.Total != total {
		t.Errorf("Status().Search = %+v, want query=%q at=%d total=%d", got, query, at, total)
	}
}

func matchedPointers(frame Frame) []string {
	pointers := make([]string, 0, len(frame.Matches))
	for _, row := range frame.Matches {
		pointers = append(pointers, frame.Lines[row].Path.String())
	}

	return pointers
}
