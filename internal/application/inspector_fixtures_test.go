package application

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// inspected opens root, points the cursor at a pointer and asks what the
// inspector would say. The cursor is set directly so that a document can be
// probed at any node without walking there first.
func inspected(t *testing.T, root domain.Node, ptr string) InspectorInfo {
	t.Helper()

	app := session(t, root)
	app.view.Cursor = pointer(t, ptr)

	return app.Inspector()
}

// inspectorTree is a document holding one of everything the inspector has to
// describe: each kind of value, a container of each kind, empty ones, an array
// whose elements are named by position, and an object with a member keyed "0"
// that a pointer alone could not tell from one of those elements.
func inspectorTree(t *testing.T) domain.Node {
	t.Helper()

	return object(t,
		member("str", text(t, "text")),
		member("num", domain.NewNumber("-12.5e3")),
		member("yes", domain.NewBool(true)),
		member("nothing", domain.NewNull()),
		member("obj", object(t,
			member("a", domain.NewNumber("1")),
			member("b", domain.NewNumber("2")),
		)),
		member("arr", domain.NewArray([]domain.Node{
			text(t, "first"),
			text(t, "second"),
		})),
		member("empty-obj", object(t)),
		member("empty-arr", domain.NewArray(nil)),
		member("digits", object(t, member("0", text(t, "keyed, not indexed")))),
		member("", text(t, "no name at all")),
	)
}
