package application

import "github.com/ytakahashi/pino/internal/domain"

// Naming says how a node is named within the container holding it.
//
// The three are exclusive and one of them always applies, which a pair of
// booleans could not say: the root is named by nothing, while a member whose
// key is the empty string is named by a key that happens to be empty. A pane
// that drew the second as the first would be denying that the member has a
// name at all.
type Naming uint8

const (
	NamedNone  Naming = iota // the root, which is a member of nothing
	NamedKey                 // a member of an object
	NamedIndex               // an element of an array
)

func (n Naming) String() string {
	switch n {
	case NamedNone:
		return "none"
	case NamedKey:
		return "key"
	case NamedIndex:
		return "index"
	default:
		return "unknown"
	}
}

// InspectorInfo is what the inspector says about the selected node.
type InspectorInfo struct {
	// Pointer locates the node, as RFC 6901 spells it: the root is the empty
	// string. How to show that is left to whoever draws the pane, in the same
	// way it is for the status bar.
	Pointer string

	// Type names the kind of the selected value. It is empty when nothing is
	// selected, which is what tells that apart from a root holding an object.
	Type string

	// Value is the scalar as it would be written, in full. Containers leave it
	// zero and report Children instead, and Container is what tells an empty
	// container from a value: both have no children.
	Value     Span
	Children  int
	Container bool

	// Label is the name the node has within its parent, and Naming says what
	// kind of name that is. Label is empty for the root, and empty as well for
	// the one member a document can hold whose key is the empty string.
	Label  string
	Naming Naming
}

// Inspector describes the selected node for the pane beside the tree.
//
// It is built here rather than in the layer that draws it because the current
// value and the number of children can only be had from the document, which
// that layer does not see. Status is in this layer for the same reason.
//
// Nothing is added to Line to carry it. A row is a row: giving it the value and
// the child count of the node it draws would turn it into a view of the node,
// and the two things drawing wants from a row would be lost among the things
// only the panes want. Resolve walks the tree as deep as the cursor and no
// further, so this answers without laying the document out at all.
func (a *App) Inspector() InspectorInfo {
	if a.doc == nil {
		return InspectorInfo{}
	}

	root := a.doc.Root()

	n, ok := domain.Resolve(root, a.view.Cursor)
	if !ok {
		return InspectorInfo{}
	}

	info := InspectorInfo{Pointer: a.view.Cursor.String(), Type: n.Kind().String()}

	if p := a.view.Cursor; !p.IsRoot() {
		info.Label, info.Naming = p.At(p.Len()-1).Token(), NamedKey

		// Whether the name is a position is asked of the document rather than
		// of the path. A pointer cannot tell "/features/0" addressing the first
		// element of an array from it addressing the member "0" of an object,
		// so a path that came from text says key to every segment; the parent
		// says which it really is. The inspector is the only place on screen
		// that can answer this, and the name of the field is the answer.
		if parent, ok := domain.Resolve(root, p.Parent()); ok && parent.Kind() == domain.KindArray {
			info.Naming = NamedIndex
		}
	}

	switch n.Kind() {
	case domain.KindObject:
		info.Container, info.Children = true, n.(*domain.Object).Len()

	case domain.KindArray:
		info.Container, info.Children = true, n.(*domain.Array).Len()

	case domain.KindString, domain.KindNumber, domain.KindBool, domain.KindNull:
		// In full, with no limit passed: a value shortened in the document is
		// read back here, which is what makes shortening one on a row safe.
		info.Value = scalarSpan(n, 0)
	}

	return info
}
