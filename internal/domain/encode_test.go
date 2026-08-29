package domain

import (
	"testing"
)

func TestEncodeWritesEachKindOfValue(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		node func(t *testing.T) Node
		want string
	}{
		"a string": {node: func(t *testing.T) Node { return str(t, "localhost") }, want: `"localhost"`},
		"a string needing quotes": {
			node: func(t *testing.T) Node { return str(t, "say \"hi\"\n") },
			want: `"say \"hi\"\n"`,
		},

		// The literal the document was read as, not a reading of it: 1.50 and
		// 1.5 are the same quantity and two different files.
		"a number":               {node: func(*testing.T) Node { return NewNumber("8080") }, want: `8080`},
		"a number keeping zeros": {node: func(*testing.T) Node { return NewNumber("1.50") }, want: `1.50`},
		"an exponent":            {node: func(*testing.T) Node { return NewNumber("1E+3") }, want: `1E+3`},
		"true":                   {node: func(*testing.T) Node { return NewBool(true) }, want: `true`},
		"false":                  {node: func(*testing.T) Node { return NewBool(false) }, want: `false`},
		"null":                   {node: func(*testing.T) Node { return NewNull() }, want: `null`},

		// A container with nothing in it takes one line, as the JSON view
		// draws it.
		"an empty object": {node: func(t *testing.T) Node { return obj(t) }, want: `{}`},
		"an empty array":  {node: func(*testing.T) Node { return NewArray(nil) }, want: `[]`},

		"an object": {
			node: func(t *testing.T) Node {
				return obj(t,
					Member{Key: "host", Value: str(t, "localhost")},
					Member{Key: "port", Value: NewNumber("8080")},
				)
			},
			want: "{\n  \"host\": \"localhost\",\n  \"port\": 8080\n}",
		},
		"an object with a key needing escapes": {
			node: func(t *testing.T) Node {
				return obj(t, Member{Key: "a\"b", Value: NewNumber("1")})
			},
			want: "{\n  \"a\\\"b\": 1\n}",
		},
		"an array": {
			node: func(t *testing.T) Node {
				return NewArray([]Node{str(t, "a"), NewNumber("1"), NewNull()})
			},
			want: "[\n  \"a\",\n  1,\n  null\n]",
		},
		"an array of objects": {
			node: func(t *testing.T) Node {
				return NewArray([]Node{
					obj(t, Member{Key: "name", Value: str(t, "first")}),
					obj(t, Member{Key: "name", Value: str(t, "second")}),
				})
			},
			want: "[\n  {\n    \"name\": \"first\"\n  },\n  {\n    \"name\": \"second\"\n  }\n]",
		},
		"a document holding every kind": {
			node: func(t *testing.T) Node { return everyKind(t) },
			want: `{
  "host": "localhost",
  "port": 8080,
  "debug": false,
  "extra": null,
  "tags": [
    "a",
    "b"
  ],
  "server": {
    "tls": {
      "port": 8443
    }
  },
  "empty": {},
  "none": []
}`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := string(Encode(tc.node(t), plainFormat())); got != tc.want {
				t.Errorf("Encode wrote\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

// The layout is the file's own, so that saving a document nobody reformatted
// does not rewrite every line of it.
func TestEncodeFollowsTheFormat(t *testing.T) {
	t.Parallel()

	document := func(t *testing.T) Node {
		t.Helper()

		return obj(t,
			Member{Key: "server", Value: obj(t, Member{Key: "port", Value: NewNumber("8080")})},
			Member{Key: "debug", Value: NewBool(false)},
		)
	}

	tests := map[string]struct {
		format Format
		want   string
	}{
		"two spaces": {
			format: Format{Indent: "  ", Newline: "\n"},
			want:   "{\n  \"server\": {\n    \"port\": 8080\n  },\n  \"debug\": false\n}",
		},
		"four spaces": {
			format: Format{Indent: "    ", Newline: "\n"},
			want:   "{\n    \"server\": {\n        \"port\": 8080\n    },\n    \"debug\": false\n}",
		},
		"tabs": {
			format: Format{Indent: "\t", Newline: "\n"},
			want:   "{\n\t\"server\": {\n\t\t\"port\": 8080\n\t},\n\t\"debug\": false\n}",
		},

		// --indent 0 asks for levels drawn no columns wide, not for a document
		// on one line: the JSON view still shows one value per row, and what
		// is saved is what is read.
		"no indent": {
			format: Format{Indent: "", Newline: "\n"},
			want:   "{\n\"server\": {\n\"port\": 8080\n},\n\"debug\": false\n}",
		},
		"CRLF": {
			format: Format{Indent: "  ", Newline: "\r\n"},
			want:   "{\r\n  \"server\": {\r\n    \"port\": 8080\r\n  },\r\n  \"debug\": false\r\n}",
		},
		"a trailing newline": {
			format: Format{Indent: "  ", Newline: "\n", TrailingNL: true},
			want:   "{\n  \"server\": {\n    \"port\": 8080\n  },\n  \"debug\": false\n}\n",
		},
		"a trailing CRLF": {
			format: Format{Indent: "  ", Newline: "\r\n", TrailingNL: true},
			want:   "{\r\n  \"server\": {\r\n    \"port\": 8080\r\n  },\r\n  \"debug\": false\r\n}\r\n",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := string(Encode(document(t), tc.format)); got != tc.want {
				t.Errorf("Encode wrote %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEncodeWritesCommentsAtTheirDocumentPositions(t *testing.T) {
	t.Parallel()

	line := func(t *testing.T, text string, ownLine bool) Comment {
		t.Helper()

		return comment(t, text, false, ownLine)
	}
	block := func(t *testing.T, text string, ownLine bool) Comment {
		t.Helper()

		return comment(t, text, true, ownLine)
	}

	tests := map[string]struct {
		node func(t *testing.T) Node
		want string
	}{
		"before the root": {
			node: func(t *testing.T) Node {
				return WithTrivia(NewNumber("1"), NewTrivia(
					[]Comment{line(t, " root", true)}, nil, nil,
				))
			},
			want: "// root\n1",
		},
		"after the root on the same line": {
			node: func(t *testing.T) Node {
				return WithTrivia(NewNumber("1"), NewTrivia(
					nil, []Comment{block(t, " after ", false)}, nil,
				))
			},
			want: "1 /* after */",
		},
		"after the root on its own line": {
			node: func(t *testing.T) Node {
				return WithTrivia(NewNumber("1"), NewTrivia(
					nil, []Comment{line(t, " after", true)}, nil,
				))
			},
			want: "1\n// after\n",
		},
		"a line comment opens a line for the comment after it": {
			node: func(t *testing.T) Node {
				return WithTrivia(NewNumber("1"), NewTrivia(
					nil,
					[]Comment{
						line(t, " first", false),
						block(t, " second ", false),
					},
					nil,
				))
			},
			want: "1 // first\n/* second */",
		},
		"before an object member": {
			node: func(t *testing.T) Node {
				return obj(t, Member{
					Key: "a", Value: NewNumber("1"),
					Trivia: NewTrivia([]Comment{line(t, " member", true)}, nil, nil),
				})
			},
			want: "{\n  // member\n  \"a\": 1\n}",
		},
		"an inline block before a member leaves the member on its own line": {
			node: func(t *testing.T) Node {
				return obj(t,
					Member{
						Key: "a", Value: NewNumber("1"),
						Trivia: NewTrivia([]Comment{block(t, " member ", false)}, nil, nil),
					},
					Member{Key: "b", Value: NewNumber("2")},
				)
			},
			want: "{ /* member */\n  \"a\": 1,\n  \"b\": 2\n}",
		},
		"between a key and its value": {
			node: func(t *testing.T) Node {
				value := WithTrivia(NewNumber("1"), NewTrivia(
					[]Comment{block(t, " why ", false)}, nil, nil,
				))

				return obj(t, Member{Key: "a", Value: value})
			},
			want: "{\n  \"a\": /* why */ 1\n}",
		},
		"after a member value and its comma": {
			node: func(t *testing.T) Node {
				value := WithTrivia(NewNumber("1"), NewTrivia(
					nil, []Comment{line(t, " trailing", false)}, nil,
				))

				return obj(t,
					Member{Key: "a", Value: value},
					Member{Key: "b", Value: NewNumber("2")},
				)
			},
			want: "{\n  \"a\": 1, // trailing\n  \"b\": 2\n}",
		},
		"after an object member": {
			node: func(t *testing.T) Node {
				return obj(t,
					Member{
						Key: "a", Value: NewNumber("1"),
						Trivia: NewTrivia(nil, []Comment{block(t, " pair ", false)}, nil),
					},
					Member{Key: "b", Value: NewNumber("2")},
				)
			},
			want: "{\n  \"a\": 1, /* pair */\n  \"b\": 2\n}",
		},
		"around array elements": {
			node: func(t *testing.T) Node {
				first := WithTrivia(NewNumber("1"), NewTrivia(
					[]Comment{line(t, " first", true)},
					[]Comment{line(t, " one", false)},
					nil,
				))

				return NewArray([]Node{first, NewNumber("2")})
			},
			want: "[\n  // first\n  1, // one\n  2\n]",
		},
		"an inline block before an element leaves the element on its own line": {
			node: func(t *testing.T) Node {
				first := WithTrivia(NewNumber("1"), NewTrivia(
					[]Comment{block(t, " first ", false)}, nil, nil,
				))

				return NewArray([]Node{first, NewNumber("2")})
			},
			want: "[ /* first */\n  1,\n  2\n]",
		},
		"inside an empty object": {
			node: func(t *testing.T) Node {
				return WithTrivia(obj(t), NewTrivia(
					nil, nil, []Comment{line(t, " pending", true)},
				))
			},
			want: "{\n  // pending\n}",
		},
		"inside an empty array on the opening line": {
			node: func(t *testing.T) Node {
				return WithTrivia(NewArray(nil), NewTrivia(
					nil, nil, []Comment{block(t, " empty ", false)},
				))
			},
			want: "[ /* empty */\n]",
		},
		"inside a populated container": {
			node: func(t *testing.T) Node {
				return WithTrivia(NewArray([]Node{NewNumber("1")}), NewTrivia(
					nil, nil, []Comment{line(t, " more later", true)},
				))
			},
			want: "[\n  1\n  // more later\n]",
		},
		"a multiline block keeps its continuation": {
			node: func(t *testing.T) Node {
				return WithTrivia(NewNumber("1"), NewTrivia(
					[]Comment{block(t, " banner\n * body ", true)}, nil, nil,
				))
			},
			want: "/* banner\n * body */\n1",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := string(Encode(tt.node(t), plainFormat())); got != tt.want {
				t.Errorf("Encode wrote %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEncodeWritesCommentsWithTheDocumentNewline(t *testing.T) {
	t.Parallel()

	node := WithTrivia(NewArray([]Node{NewNumber("1")}), NewTrivia(
		[]Comment{comment(t, " root", false, true)},
		nil,
		[]Comment{comment(t, " inside ", true, true)},
	))
	format := Format{Indent: "\t", Newline: "\r\n", TrailingNL: true}
	want := "// root\r\n[\r\n\t1\r\n\t/* inside */\r\n]\r\n"

	if got := string(Encode(node, format)); got != want {
		t.Errorf("Encode wrote %q, want %q", got, want)
	}
}

func TestEncodePanicsWhenInsideCommentsHaveNoContainer(t *testing.T) {
	t.Parallel()

	inside := NewTrivia(nil, nil, []Comment{comment(t, " impossible", false, true)})
	tests := map[string]func(t *testing.T) Node{
		"a scalar": func(*testing.T) Node {
			return WithTrivia(NewNull(), inside)
		},
		"an object member": func(t *testing.T) Node {
			return obj(t, Member{Key: "a", Value: NewNull(), Trivia: inside})
		},
	}

	for name, build := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if recover() == nil {
					t.Error("Encode returned normally, want a panic")
				}
			}()

			Encode(build(t), plainFormat())
		})
	}
}

func TestEncoderDoesNotIndentTheSameOpenLineTwice(t *testing.T) {
	t.Parallel()

	e := encoder{format: plainFormat()}
	e.startLine(1)
	e.startLine(1)
	e.writeString("1")

	if got := e.buf.String(); got != "  1" {
		t.Errorf("encoder wrote %q, want one level of indentation", got)
	}
}

// A document with nothing in it still ends where the format says, which is
// the one case with no line to end before the last one.
func TestEncodeEndsARootScalarAndAnEmptyDocumentToo(t *testing.T) {
	t.Parallel()

	format := Format{Indent: "  ", Newline: "\n", TrailingNL: true}

	tests := map[string]struct {
		node func(t *testing.T) Node
		want string
	}{
		"an empty object": {node: func(t *testing.T) Node { return obj(t) }, want: "{}\n"},
		"an empty array":  {node: func(*testing.T) Node { return NewArray(nil) }, want: "[]\n"},
		"a root number":   {node: func(*testing.T) Node { return NewNumber("42") }, want: "42\n"},
		"a root string":   {node: func(t *testing.T) Node { return str(t, "x") }, want: "\"x\"\n"},
		"a root null":     {node: func(*testing.T) Node { return NewNull() }, want: "null\n"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := string(Encode(tc.node(t), format)); got != tc.want {
				t.Errorf("Encode wrote %q, want %q", got, tc.want)
			}
		})
	}
}

// Saving happens while the document is being edited, so writing one version
// must leave the tree it was written from exactly as it was: the same nodes,
// shared with every other version, and the same comments on them.
func TestEncodeLeavesTheTreeAlone(t *testing.T) {
	t.Parallel()

	d := newTree(t)
	before := Encode(d.root, plainFormat())

	if got := nodeAt(t, d.root, "/server"); got != Node(d.server) {
		t.Error("encoding replaced a subtree")
	}

	if got := nodeAt(t, d.root, "/features/1"); got != Node(d.second) {
		t.Error("encoding replaced a node deep in the document")
	}

	if got := string(Encode(d.root, plainFormat())); got != string(before) {
		t.Error("encoding the same document twice wrote two different documents")
	}

	commentedRoot, _ := commented(t)
	elsewhere, _ := commented(t)

	Encode(commentedRoot, plainFormat())

	if !Equal(commentedRoot, elsewhere) {
		t.Error("encoding changed the document it was given")
	}
}

// A *Null holding a nil pointer has no field to read, so it would otherwise
// be written as a legitimate null: a value that went missing on the way here
// would be saved as one the user appeared to have typed.
func TestEncodePanicsOnANodeThatIsNotThere(t *testing.T) {
	t.Parallel()

	nodes := typedNils()
	nodes["nothing at all"] = nil

	for name, missing := range nodes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if recover() == nil {
					t.Errorf("Encode of a missing %s returned normally, want a panic", name)
				}
			}()

			Encode(missing, DefaultFormat())
		})
	}
}

// A layout that could not produce JSON is a mistake in pino rather than
// something a document or a user can cause: DefaultFormat, DetectFormat and
// the checked command line flag are the only ways one is built.
func TestEncodePanicsOnAFormatThatWouldNotProduceJSON(t *testing.T) {
	t.Parallel()

	tests := map[string]Format{
		"the zero value":           {},
		"no newline":               {Indent: "  ", Newline: ""},
		"a lone carriage return":   {Indent: "  ", Newline: "\r"},
		"a newline that is text":   {Indent: "  ", Newline: "; "},
		"an indent of text":        {Indent: "x", Newline: "\n"},
		"an indent ending in text": {Indent: "  x", Newline: "\n"},
	}

	for name, format := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if recover() == nil {
					t.Errorf("Encode returned normally with %#v, want a panic", format)
				}
			}()

			Encode(NewNull(), format)
		})
	}
}

func TestQuoteStringEscapesAndSurroundsTheValue(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in   string
		want string
	}{
		"empty":                {in: "", want: `""`},
		"plain text":           {in: "localhost", want: `"localhost"`},
		"a quote":              {in: `say "hi"`, want: `"say \"hi\""`},
		"a backslash":          {in: `C:\tmp`, want: `"C:\\tmp"`},
		"the short escapes":    {in: "\b\f\n\r\t", want: `"\b\f\n\r\t"`},
		"a control character":  {in: "a\x00b", want: `"a\u0000b"`},
		"the last control one": {in: "\x1f", want: `"\u001f"`},

		// U+007F is not a JSON control character and stays as it is, as does
		// text outside ASCII: escaping either would only make it unreadable.
		"delete":     {in: "\x7f", want: "\"\x7f\""},
		"kanji":      {in: "設定", want: `"設定"`},
		"an emoji":   {in: "🌲", want: `"🌲"`},
		"a solidus":  {in: "a/b", want: `"a/b"`},
		"whitespace": {in: " ", want: `" "`},

		// Text that is valid UTF-8 is written through whatever it says, and
		// U+FFFD is valid. Bytes that are not valid never get this far:
		// NewString and NewObject refuse them.
		"the replacement character": {in: "�", want: "\"�\""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := QuoteString(tc.in); got != tc.want {
				t.Errorf("QuoteString(%q) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestEscapeStringEscapesWithoutSurroundingQuotes(t *testing.T) {
	t.Parallel()

	for name, tc := range escapeCases() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := EscapeString(tc.in); got != tc.want {
				t.Errorf("EscapeString(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The renderer builds a shortened value as a quote, the escaped beginning of
// the string, a marker and a closing quote. That only produces the same text
// as an unshortened value if the two functions agree on everything but the
// quotes.
func TestQuoteStringIsEscapeStringInQuotes(t *testing.T) {
	t.Parallel()

	for name, tc := range escapeCases() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			want := `"` + EscapeString(tc.in) + `"`
			if got := QuoteString(tc.in); got != want {
				t.Errorf("QuoteString(%q) = %s, want %s", tc.in, got, want)
			}
		})
	}
}
