// Package testdocs holds the corpus of documents the application layer is
// tested against.
//
// It is a package rather than a fixture file because two packages read it: the
// renderers draw every document into a golden file, while the session replays
// edits and view switches over the same trees. Go cannot share a fixture across
// packages, and a corpus written out twice would let a document added for one
// of them go unchecked by the other.
//
// The drawing options are spelled out here rather than held as a
// documentview.Options: an in-package test of documentview cannot import a
// package that imports documentview, so this one depends on domain alone.
package testdocs

import (
	"github.com/ytakahashi/pino/internal/domain"
)

// Document is one document of the corpus, together with the options it is
// drawn under. The zero value of the options draws everything, in full.
type Document struct {
	Root      domain.Node
	Collapsed []string // JSON Pointers of the containers folded away
	MaxStrLen int      // 0 means strings are shown in full
}

// Documents is the corpus.
//
// A document belongs here when it says something about how a structure is
// drawn or edited; one that exists to exercise a single helper belongs next to
// that helper's test.
func Documents() map[string]Document {
	return map[string]Document{
		// Comments at every rendering boundary: around the root, around an
		// object member and its value, around an array element, and inside both
		// populated and otherwise empty containers.
		"comments": {Root: WithTrivia(Object(
			MemberWithTrivia(
				"enabled",
				WithTrivia(domain.NewBool(true), Trivia(
					[]domain.Comment{Comment(" value ", true, false)}, nil, nil,
				)),
				Trivia(
					[]domain.Comment{Comment(" member", false, true)},
					[]domain.Comment{Comment(" trailing", false, false)}, nil,
				),
			),
			MemberWithTrivia("items", WithTrivia(domain.NewArray([]domain.Node{
				WithTrivia(Text("first"), Trivia(
					[]domain.Comment{Comment(" first item ", true, false)},
					[]domain.Comment{Comment(" item note ", true, false)}, nil,
				)),
				WithTrivia(Text("second"), Trivia(
					[]domain.Comment{Comment(" second item", false, true)}, nil, nil,
				)),
			}), Trivia(nil, nil, []domain.Comment{Comment(" more items", false, true)})), Trivia(
				nil, []domain.Comment{Comment(" items complete ", true, false)}, nil,
			)),
			MemberWithTrivia("nested", Object(
				Member("leaf", domain.NewNull()),
			), Trivia(nil, []domain.Comment{Comment(" nested complete ", true, false)}, nil)),
			Member("empty-object", WithTrivia(Object(), Trivia(
				nil, nil, []domain.Comment{Comment(" banner\n * kept as written ", true, true)},
			))),
			Member("empty-array", WithTrivia(domain.NewArray(nil), Trivia(
				nil, nil, []domain.Comment{Comment(" empty", false, true)},
			))),
		), Trivia(
			[]domain.Comment{Comment(" document", false, true)},
			[]domain.Comment{Comment(" end", false, true)}, nil,
		))},

		// Containers within containers, down to an array of strings.
		"nested": {Root: Object(
			Member("server", Object(
				Member("host", Text("localhost")),
				Member("port", domain.NewNumber("8080")),
				Member("features", domain.NewArray([]domain.Node{
					Text("search"),
					Text("history"),
				})),
			)),
		)},

		// Every kind that occupies a single row, so that each Role appears.
		"scalars": {Root: Object(
			Member("str", Text("text")),
			Member("num", domain.NewNumber("-12.5e3")),
			Member("yes", domain.NewBool(true)),
			Member("no", domain.NewBool(false)),
			Member("nothing", domain.NewNull()),
		)},

		// Empty containers, including one that is not the last member and so
		// has to carry a comma.
		"empty": {Root: Object(
			Member("obj", Object()),
			Member("arr", domain.NewArray(nil)),
			Member("outer", Object(
				Member("inner", Object()),
			)),
			Member("last", domain.NewNumber("1")),
		)},

		// A document that is nothing but an empty container. It is the one
		// document whose root offers nothing to unfold.
		"empty-root": {Root: Object()},

		// An array at the root, holding objects and an array.
		"array-of-objects": {Root: domain.NewArray([]domain.Node{
			Object(
				Member("id", domain.NewNumber("1")),
				Member("tags", domain.NewArray([]domain.Node{Text("a")})),
			),
			Object(Member("id", domain.NewNumber("2"))),
		})},

		// A document that is a single value.
		"root-scalar": {Root: Text("just a string")},

		// Text that has to be escaped, in the row and in the pointer. The
		// control characters matter twice over: unescaped, they would reach
		// the terminal.
		"escapes": {Root: Object(
			Member("a/b", Text(`say "hi"`)),
			Member("c~d", Text("tab\there")),
			Member("ctrl", Text("nul:\x00 esc:\x1b")),
		)},

		// Keys that decide how a view names a member. The JSON view quotes
		// every one of them; a view that does not has to tell the ones that
		// read as themselves from the ones that would reach the terminal or
		// leave the row unreadable.
		"keys": {Root: Object(
			Member("", Text("no name at all")),
			Member("plain", Text("a")),
			Member("with space", Text("b")),
			Member("設定", Text("c")),
			Member(`say "hi"`, Text("d")),
			Member(`back\slash`, Text("e")),
			Member("tab\there", Text("f")),
			Member("nl\nhere", Text("g")),
		)},

		// Folded containers: one before its last sibling and one after it, so
		// that both sides of the separating comma are covered; one nested
		// inside a container still open; an array as well as an object; and a
		// key whose pointer has to be escaped to find it in the set.
		"collapsed": {
			Root: Object(
				Member("server", Object(
					Member("cache", Object(Member("ttl", domain.NewNumber("60")))),
					Member("host", Text("localhost")),
				)),
				Member("features", domain.NewArray([]domain.Node{
					Text("search"),
					Text("history"),
				})),
				Member("a/b", Object(Member("x", domain.NewNumber("1")))),
				Member("opts", Object(Member("y", domain.NewNumber("2")))),
			),
			Collapsed: []string{"/server/cache", "/features", "/a~1b", "/opts"},
		},

		// The whole document folded into one row. The root is keyed by the
		// empty pointer, which is the one entry easy to get wrong.
		"collapsed-root": {
			Root: Object(
				Member("server", Object(Member("host", Text("localhost")))),
				Member("port", domain.NewNumber("8080")),
			),
			Collapsed: []string{""},
		},

		// Entries in the set that cannot fold anything: empty containers,
		// which say as much unfolded, a scalar, and a node that is not there.
		"collapsed-ignored": {
			Root: Object(
				Member("obj", Object()),
				Member("arr", domain.NewArray(nil)),
				Member("num", domain.NewNumber("1")),
			),
			Collapsed: []string{"/obj", "/arr", "/num", "/missing"},
		},

		// Values shortened at a limit small enough to read in the golden: one
		// under it, one exactly on it, one over; a key long enough to be cut
		// if keys were cut; text whose runes are wider than a byte; and a cut
		// that lands where an escape begins.
		"elided": {
			Root: Object(
				Member("short", Text("abcd")),
				Member("exact", Text("abcde")),
				Member("over", Text("abcdef")),
				Member("keeps-its-full-name", Text("x")),
				Member("kanji", Text("設定ファイルです")),
				Member("escaped", Text("ab\ncdef")),
				Member("quotes", Text(`ab"cdef`)),
			),
			MaxStrLen: 5,
		},
	}
}

// Sample is a document with a container holding a container, an array of
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
func Sample() domain.Node {
	return Object(
		Member("name", Text("pino")),
		Member("server", Object(
			Member("host", Text("localhost")),
			Member("ports", domain.NewArray([]domain.Node{
				domain.NewNumber("8080"),
				domain.NewNumber("8443"),
			})),
			Member("tls", domain.NewBool(true)),
		)),
		Member("debug", domain.NewBool(false)),
	)
}

// Object is a JSON object holding members.
//
// The constructors are wrapped so that a fixture reads as the document it
// stands for. They panic rather than report, because what they reject —
// duplicate keys, text that is not UTF-8 — is a mistake in a fixture written
// here rather than something a test could go on to check.
func Object(members ...domain.Member) domain.Node {
	o, err := domain.NewObject(members)
	if err != nil {
		panic("testdocs: " + err.Error())
	}

	return o
}

// Member names a value within an object.
func Member(key string, value domain.Node) domain.Member {
	return domain.Member{Key: key, Value: value}
}

// MemberWithTrivia names a value and attaches comments to the member itself.
func MemberWithTrivia(key string, value domain.Node, trivia domain.Trivia) domain.Member {
	return domain.Member{Key: key, Value: value, Trivia: trivia}
}

// WithTrivia attaches comments to a fixture node.
func WithTrivia(node domain.Node, trivia domain.Trivia) domain.Node {
	return domain.WithTrivia(node, trivia)
}

// Trivia builds the three comment positions in their document order.
func Trivia(before, after, inside []domain.Comment) domain.Trivia {
	return domain.NewTrivia(before, after, inside)
}

// Comment builds validated fixture text. Invalid text is a broken fixture,
// not a condition a test using it can recover from.
func Comment(text string, block, ownLine bool) domain.Comment {
	comment, err := domain.NewComment(text, block, ownLine)
	if err != nil {
		panic("testdocs: " + err.Error())
	}

	return comment
}

// Text is a JSON string holding v.
func Text(v string) domain.Node {
	s, err := domain.NewString(v)
	if err != nil {
		panic("testdocs: " + err.Error())
	}

	return s
}

// Path builds a Path the way a renderer walks a tree.
func Path(segs ...domain.Segment) domain.Path {
	p := domain.Path{}
	for _, s := range segs {
		p = p.Child(s)
	}

	return p
}

// Folded is a set of folded nodes, keyed the way the drawing options want it.
func Folded(pointers ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(pointers))
	for _, p := range pointers {
		set[p] = struct{}{}
	}

	return set
}
