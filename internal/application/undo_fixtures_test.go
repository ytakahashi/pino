package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// commitEdit makes an edit current, the way the editing flow will once it is
// wired up: the new tree, a version to come back to, and the folded set
// following where things moved.
func commitEdit(t *testing.T, a *App, res domain.EditResult, label string) {
	t.Helper()

	a.doc.Replace(res.Root)
	a.history.Push(Revision{Root: res.Root, Cursor: res.Cursor, Label: label})
	a.view.Apply(res)
	a.settle(a.render())
}

// featureList is an array of containers, which is the one shape where a fold
// has to move: deleting an element brings the ones after it down, and a fold
// sits on one of those.
//
//	{ "features": [ {"name": "first"}, {"name": "second"} ] }
func featureList(t *testing.T) domain.Node {
	t.Helper()

	return object(t,
		member("features", domain.NewArray([]domain.Node{
			object(t, member("name", text(t, "first"))),
			object(t, member("name", text(t, "second"))),
		})),
	)
}
