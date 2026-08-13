package domain

import (
	"slices"
	"strconv"
	"strings"
)

// EditResult is what an edit produced: the new tree, where the cursor should
// be left, and how the paths of the surviving nodes moved.
//
// Root is the original root when the edit changed nothing, which is how the
// layer above tells "nothing happened" from "something did" without comparing
// two trees.
type EditResult struct {
	Root    Node
	Cursor  Path
	Renames []PathMap

	// Removed is the paths whose subtrees the edit took out of the document.
	// It is a prefix, like PathMap: the node named and everything beneath it
	// are gone.
	//
	// A holder of paths cannot work this out for itself by asking which of
	// them still resolve. Taking an element out of an array moves the next one
	// into its place, so the path of what was deleted goes on resolving — to a
	// different node. A collapsed set that followed an edit by resolution
	// alone would keep that path and fold the wrong row. Only the edit can
	// tell a node that is gone from a node that moved.
	//
	// It applies before Renames: the paths it drops are the ones that pointed
	// into the subtree as it stood before the edit, so the array positions in
	// it are the ones from before anything shifted.
	Removed []Path
}

// PathMap moves a subtree from one path to another.
//
// It is a prefix: the node at From moves to To, and so does everything beneath
// it. A path is therefore moved when it equals From or begins with From
// followed by a separator — the second condition has to be spelled that way,
// or renaming "/a" would claim "/ab" as well.
//
// The maps of one EditResult are applied simultaneously, never one after
// another. Inserting into an array yields {1->2} and {2->3}, and applying
// those in turn would carry "/1" through "/2" and on to "/3"; a single pass
// that reads the old set and writes a new one cannot go wrong that way,
// whatever order the maps are in.
type PathMap struct{ From, To Path }

// RewritePointer is the JSON Pointer text as an edit left it.
//
// The maps of one EditResult apply at once, and this is what that means in
// practice: a pointer is looked at one time, against the whole set, so the
// answer does not depend on the order they are in. Rewriting a set of pointers
// therefore means calling this once for each of them, never applying one map
// to the set and then the next.
//
// At most one map can cover a pointer. Within a single edit the paths that
// move are siblings of one another, or there is only one of them, so there is
// no case where the first match is a choice.
//
// It takes text rather than a Path because the sets that have to follow an
// edit are keyed by pointer: a Path holds a slice and cannot be a map key. How
// a pointer is spelled is knowledge about JSON Pointer, which lives here, so
// the layers holding such a set ask rather than spell it out again.
func RewritePointer(maps []PathMap, pointer string) string {
	for _, m := range maps {
		if rest, ok := under(m.From.String(), pointer); ok {
			return m.To.String() + rest
		}
	}

	return pointer
}

// PointerRemoved reports whether pointer named something an edit took out.
//
// It is asked before the renames are applied, since the paths an edit removes
// are the ones from the document as it stood before it.
func PointerRemoved(removed []Path, pointer string) bool {
	for _, p := range removed {
		if _, ok := under(p.String(), pointer); ok {
			return true
		}
	}

	return false
}

// under reports whether pointer names prefix or something beneath it, and
// returns the text that follows prefix.
//
// The separator in the second case is what keeps a rename of "/a" off "/ab":
// to be beneath a path, the text has to go on with a new token rather than
// with more of the last one. The root is the empty pointer and so is under
// nothing and above everything, which is the answer both callers want.
func under(prefix, pointer string) (string, bool) {
	if pointer == prefix {
		return "", true
	}

	if strings.HasPrefix(pointer, prefix+"/") {
		return pointer[len(prefix):], true
	}

	return "", false
}

// EditError reports an edit the document cannot take.
//
// The five operations return an error rather than panicking because what they
// refuse depends on the document: a path that no longer resolves, a position
// past the end of an array, a key another member already has. Undo and a stale
// cursor can produce all three between them, so they are not necessarily a
// mistake by the caller. A negative index or a missing node still panics, for
// the reason IndexSegment gives — no document can ask for either.
type EditError struct {
	Path   Path
	Reason string
}

func (e *EditError) Error() string {
	if e.Path.IsRoot() {
		return "cannot edit the document root: " + e.Reason
	}

	return "cannot edit " + strconv.Quote(e.Path.String()) + ": " + e.Reason
}

// SetValue replaces the value at p.
//
// The tree comes back unchanged when the new value is the one already there,
// which is what lets the layer above tell "nothing happened" from "something
// did" by comparing two pointers.
func SetValue(root Node, p Path, v Node) (EditResult, error) {
	if isNilNode(v) {
		panic("domain: no value to set at " + strconv.Quote(p.String()))
	}

	var removed []Path

	next, err := replaceNode(root, p, func(old Node) (Node, error) {
		if sameValue(old, v) {
			return old, nil
		}

		// Whatever the old value held goes with it. Nothing installs a
		// populated container here yet, so no path can survive under p today;
		// saying so anyway is what keeps that true when copy and paste starts
		// to.
		if hasChildren(old) {
			removed = []Path{p}
		}

		return withTrivia(v, old.Trivia()), nil
	})
	if err != nil {
		return EditResult{}, err
	}

	return EditResult{Root: next, Cursor: p, Removed: removed}, nil
}

// Rename changes the key of the object member at p.
//
// The member keeps its place among its siblings, because the order of the keys
// is part of the document and renaming one is not a reordering. The subtree
// beneath it moves with it, which is the single PathMap that comes back.
//
// It refuses a key another member already has: two members of the same object
// with one key would make a path name two nodes, which is the invariant the
// cursor, the collapsed set and undo all rest on.
func Rename(root Node, p Path, key string) (EditResult, error) {
	if p.IsRoot() {
		return EditResult{}, &EditError{Path: p, Reason: "not an object member"}
	}

	parent := p.Parent()
	seg := p.At(p.Len() - 1)

	var result EditResult

	next, err := replaceNode(root, parent, func(c Node) (Node, error) {
		if c.Kind() != KindObject {
			return nil, &EditError{Path: p, Reason: "not an object member"}
		}

		o := c.(*Object)

		_, pos, ok := childAt(o, seg)
		if !ok {
			return nil, &EditError{Path: p, Reason: "no such node"}
		}

		// The cursor is spelled from the new key either way, so that it names
		// the member the same way whether or not the name changed.
		result.Cursor = parent.Child(KeySegment(key))

		if o.At(pos).Key == key {
			return o, nil
		}

		members := membersOf(o)

		// Only the key changes. The value is shared and the comments around
		// the member are its own, so both come through untouched.
		members[pos].Key = key

		next, err := rebuildObject(o, members)
		if err != nil {
			return nil, err
		}

		result.Renames = []PathMap{{From: p, To: result.Cursor}}

		return next, nil
	})
	if err != nil {
		return EditResult{}, err
	}

	result.Root = next

	return result, nil
}

// Insert puts m into the container at parent, as its at-th child.
//
// at may be the length of the container, which appends. Anything further out
// is refused rather than clamped: a position is worked out from the cursor,
// and one that has drifted past the end means the caller is looking at a
// document that has since changed.
//
// A Member is taken whichever kind the container is, and an array refuses a
// non-empty Key rather than ignoring it. An argument documented as ignored is
// one that stops being ignored the day someone reads the field.
func Insert(root Node, parent Path, at int, m Member) (EditResult, error) {
	var result EditResult

	next, err := replaceNode(root, parent, func(c Node) (Node, error) {
		switch c.Kind() {
		case KindObject:
			o := c.(*Object)
			if at < 0 || at > o.Len() {
				return nil, &EditError{Path: parent, Reason: "index out of range"}
			}

			next, err := rebuildObject(o, slices.Insert(membersOf(o), at, m))
			if err != nil {
				return nil, err
			}

			// The keys of the members already there do not change, so nothing
			// moves however far up the list the new one goes.
			result.Cursor = parent.Child(KeySegment(m.Key))

			return next, nil

		case KindArray:
			a := c.(*Array)
			if m.Key != "" {
				return nil, &EditError{
					Path:   parent,
					Reason: "an array element cannot have a key",
				}
			}

			if at < 0 || at > a.Len() {
				return nil, &EditError{Path: parent, Reason: "index out of range"}
			}

			result.Cursor = parent.Child(IndexSegment(at))
			result.Renames = shiftFrom(parent, at, a.Len(), 1)

			return rebuildArray(a, slices.Insert(elementsOf(a), at, m.Value)), nil

		case KindString, KindNumber, KindBool, KindNull:
			// A value with no children cannot be given one. This is the same
			// line l draws when it refuses to descend into a primitive.
			return nil, &EditError{Path: parent, Reason: "not a container"}

		default:
			return nil, &EditError{Path: parent, Reason: "not a container"}
		}
	})
	if err != nil {
		return EditResult{}, err
	}

	result.Root = next

	return result, nil
}

// Delete removes the node at p, and says where to stand afterwards: the next
// sibling, or the previous one, or the container itself.
//
// The next sibling comes first so that holding d down deletes one node after
// another from the same row, the way dd does in vim.
//
// The root cannot be deleted: what would be left is not a document. Emptying
// it is a change of type to an object or an array, which is a different
// operation and is undone the same way.
func Delete(root Node, p Path) (EditResult, error) {
	if p.IsRoot() {
		return EditResult{}, &EditError{
			Path:   p,
			Reason: "the root cannot be deleted",
		}
	}

	parent := p.Parent()
	seg := p.At(p.Len() - 1)

	// The node and its subtree leave the document. In an array the position it
	// occupied is taken over by what followed, so nothing downstream could work
	// this out by resolving p afterwards.
	result := EditResult{Removed: []Path{p}}

	next, err := replaceNode(root, parent, func(c Node) (Node, error) {
		_, pos, ok := childAt(c, seg)
		if !ok {
			return nil, &EditError{Path: p, Reason: "no such node"}
		}

		switch c.Kind() {
		case KindObject:
			o := c.(*Object)

			// The keys that remain are the keys that were there, so nothing
			// moves and there is nothing to rename.
			result.Cursor = neighbourKey(parent, o, pos)

			next, err := rebuildObject(o, slices.Delete(membersOf(o), pos, pos+1))
			if err != nil {
				return nil, err
			}

			return next, nil

		case KindArray:
			a := c.(*Array)

			result.Cursor = neighbourIndex(parent, a.Len()-1, pos)
			result.Renames = shiftFrom(parent, pos+1, a.Len(), -1)

			return rebuildArray(a, slices.Delete(elementsOf(a), pos, pos+1)), nil

		case KindString, KindNumber, KindBool, KindNull:
			// Unreachable: childAt only finds a position inside a container.
			return nil, &EditError{Path: parent, Reason: "not a container"}

		default:
			return nil, &EditError{Path: parent, Reason: "not a container"}
		}
	})
	if err != nil {
		return EditResult{}, err
	}

	result.Root = next

	return result, nil
}

// ChangeType replaces the node at p with one of kind k, carrying its value
// over where Convert can read one.
//
// Choosing the kind the node already has changes nothing: Convert hands the
// node itself back, and the tree comes out as the tree that went in. Choosing
// a primitive for a container discards the children, which is why the layer
// above asks before calling.
func ChangeType(root Node, p Path, k Kind) (EditResult, error) {
	var removed []Path

	next, err := replaceNode(root, p, func(old Node) (Node, error) {
		converted, err := Convert(old, k)
		if err != nil {
			return nil, err
		}

		if converted == old {
			return old, nil
		}

		// The children of a container do not come through a change of type,
		// so every path that named one is a path to nothing.
		if hasChildren(old) {
			removed = []Path{p}
		}

		return withTrivia(converted, old.Trivia()), nil
	})
	if err != nil {
		return EditResult{}, err
	}

	return EditResult{Root: next, Cursor: p, Removed: removed}, nil
}

// neighbourKey is the member of o a reader should be left on once the member
// at pos is gone: the one after it, the one before it, or o itself.
func neighbourKey(parent Path, o *Object, pos int) Path {
	switch {
	case pos+1 < o.Len():
		return parent.Child(KeySegment(o.At(pos + 1).Key))
	case pos > 0:
		return parent.Child(KeySegment(o.At(pos - 1).Key))
	default:
		return parent
	}
}

// neighbourIndex is the same for an array, in terms of the length it will have
// once the element at pos is gone. The element that follows moves down into
// pos, so the position to stand on is pos itself.
func neighbourIndex(parent Path, remaining, pos int) Path {
	switch {
	case pos < remaining:
		return parent.Child(IndexSegment(pos))
	case pos > 0:
		return parent.Child(IndexSegment(pos - 1))
	default:
		return parent
	}
}

// shiftFrom is the renames for the elements of an array from first up to but
// not including end, each moving by by.
//
// Only arrays produce these: an object addresses its members by key, and
// removing one leaves the keys of the others alone.
func shiftFrom(parent Path, first, end, by int) []PathMap {
	if first >= end {
		return nil
	}

	maps := make([]PathMap, 0, end-first)
	for i := first; i < end; i++ {
		maps = append(maps, PathMap{
			From: parent.Child(IndexSegment(i)),
			To:   parent.Child(IndexSegment(i + by)),
		})
	}

	return maps
}

// replaceNode rebuilds the ancestors of p so that the node it addresses is
// replaced by what edit returns, sharing every subtree not on the way.
//
// edit is handed the node at p and returns the one to put in its place, which
// is where the operations differ from each other. Everything else — walking
// down, rebuilding on the way back up, keeping the original root when nothing
// changed, and the error for a path that leads nowhere — is the same for all
// of them and is written here once.
//
// Rebuilding goes through NewObject and NewArray, so the checks that keep a
// tree well formed apply to an edited document exactly as they do to one just
// read: an Object holds its members and an index over them, and a rebuild that
// updated only one of the two would be a document whose paths no longer name
// what they resolve to.
//
// The root is not a special case. It is the node at the empty path, and edit
// is handed it directly.
func replaceNode(root Node, p Path, edit func(n Node) (Node, error)) (Node, error) {
	if isNilNode(root) {
		return nil, &EditError{Path: p, Reason: "no such node"}
	}

	// The way down, remembering each container passed through and where the
	// path went in it, so that the way back up needs no second search.
	type step struct {
		parent Node
		pos    int
	}

	steps := make([]step, 0, p.Len())
	n := root

	for _, seg := range p.All() {
		child, pos, ok := childAt(n, seg)
		if !ok {
			return nil, &EditError{Path: p, Reason: "no such node"}
		}

		steps = append(steps, step{parent: n, pos: pos})
		n = child
	}

	next, err := edit(n)
	if err != nil {
		return nil, err
	}

	// The node is the one that was already there, so no ancestor of it changed
	// either and the document is the document it was. Returning the original
	// root propagates that all the way out, where a single pointer comparison
	// stands for "the edit was a no-op": no version is pushed, the unsaved
	// mark does not light up, and nothing follows the paths that did not move.
	if next == n {
		return root, nil
	}

	for i := len(steps) - 1; i >= 0; i-- {
		next, err = withChild(steps[i].parent, steps[i].pos, next)
		if err != nil {
			return nil, err
		}
	}

	return next, nil
}

// withChild is parent with the child at pos replaced by child.
func withChild(parent Node, pos int, child Node) (Node, error) {
	switch parent.Kind() {
	case KindObject:
		o := parent.(*Object)

		members := membersOf(o)

		// Only the value is replaced. The key and the comments around the
		// member belong to the member rather than to what it holds, so they
		// survive an edit of the value.
		members[pos].Value = child

		next, err := rebuildObject(o, members)
		if err != nil {
			return nil, err
		}

		return next, nil

	case KindArray:
		a := parent.(*Array)

		elements := elementsOf(a)
		elements[pos] = child

		return rebuildArray(a, elements), nil

	case KindString, KindNumber, KindBool, KindNull:
		// Unreachable: childAt only finds a position inside a container.
		return nil, &EditError{Reason: "not a container"}

	default:
		return nil, &EditError{Reason: "not a container"}
	}
}

// rebuildObject is NewObject with the comments around from carried over.
//
// Rebuilding a container is not a change to that container: path copying
// replaces it because a member of it was edited, or because a member was added
// or taken away, and the comments written around the container itself are
// about none of that. This and rebuildArray are the copy sites Trivia was put
// on every node for, so that adding it later would not mean auditing them.
func rebuildObject(from *Object, members []Member) (*Object, error) {
	next, err := NewObject(members)
	if err != nil {
		return nil, err
	}

	next.trivia = from.trivia

	return next, nil
}

// rebuildArray is NewArray with the comments around from carried over.
func rebuildArray(from *Array, elements []Node) *Array {
	next := NewArray(elements)
	next.trivia = from.trivia

	return next
}

// withTrivia is n carrying t.
//
// The comments around a value sit at a place in the document rather than on
// the value that happens to occupy it, so putting a new value there keeps
// them. In an object they belong to the member and travel with it; for an
// array element and for the root there is nowhere else for them to be, which
// is why this exists.
//
// It returns n unchanged when there is nothing to carry, so an edit that is
// not a change stays recognisable by identity.
//
// The copy is shallow, which is safe because every field of a node is fixed
// once built: the constructors copy what they are given, and nothing writes to
// a node afterwards. The members, elements and index of the copy may therefore
// be the ones n holds.
func withTrivia(n Node, t Trivia) Node {
	if t.IsEmpty() {
		return n
	}

	switch n.Kind() {
	case KindObject:
		next := *n.(*Object)
		next.trivia = t

		return &next

	case KindArray:
		next := *n.(*Array)
		next.trivia = t

		return &next

	case KindString:
		next := *n.(*String)
		next.trivia = t

		return &next

	case KindNumber:
		next := *n.(*Number)
		next.trivia = t

		return &next

	case KindBool:
		next := *n.(*Bool)
		next.trivia = t

		return &next

	case KindNull:
		next := *n.(*Null)
		next.trivia = t

		return &next

	default:
		panic("domain: unknown kind " + strconv.Itoa(int(n.Kind())))
	}
}

// membersOf is a copy of o's members, for an operation to reshape before
// handing it back to NewObject.
func membersOf(o *Object) []Member {
	members := make([]Member, 0, o.Len())
	for _, m := range o.All() {
		members = append(members, m)
	}

	return members
}

// elementsOf is a copy of a's elements, for the same purpose.
func elementsOf(a *Array) []Node {
	elements := make([]Node, 0, a.Len())
	for _, e := range a.All() {
		elements = append(elements, e)
	}

	return elements
}

// hasChildren reports whether anything would be lost with n.
//
// It is the question Removed asks, and it is not CountDescendants: a subtree
// is discarded whole, so its size makes no difference to whether the paths
// into it have to go. Counting would walk the whole of it to answer, and an
// edit that replaces a large container would pay for that twice — the layer
// above has already counted, to say out loud how much a confirmation is about
// to discard.
func hasChildren(n Node) bool {
	switch n.Kind() {
	case KindObject:
		return n.(*Object).Len() > 0

	case KindArray:
		return n.(*Array).Len() > 0

	case KindString, KindNumber, KindBool, KindNull:
		return false

	default:
		return false
	}
}

// sameValue reports whether replacing a with b would change the document.
//
// Scalars are compared by what they hold; containers only by identity, since
// the edits that install a container install a fresh empty one or a subtree
// taken from elsewhere, and a deep comparison would cost the size of the tree
// to answer a question that is being asked to save a keystroke's worth of
// work.
//
// Numbers are compared by their literal, not by the value it denotes: 1.50 and
// 1.5 are the same quantity but not the same document, and pino writes back
// what it was given.
func sameValue(a, b Node) bool {
	if a == b {
		return true
	}

	if a.Kind() != b.Kind() {
		return false
	}

	switch a.Kind() {
	case KindString:
		return a.(*String).Value() == b.(*String).Value()

	case KindNumber:
		return a.(*Number).Raw() == b.(*Number).Raw()

	case KindBool:
		return a.(*Bool).Value() == b.(*Bool).Value()

	case KindNull:
		// There is only one null, however many nodes spell it.
		return true

	case KindObject, KindArray:
		// Identity, which the comparison above has already ruled out.
		return false

	default:
		return false
	}
}
