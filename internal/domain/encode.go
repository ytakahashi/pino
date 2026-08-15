package domain

import (
	"bytes"
	"strconv"
	"strings"
)

// Encode writes n as the bytes of a JSON document, laid out as f says.
//
// This is what a file is written from, so what it produces is the whole of
// what pino saves. It is a pure function of the tree and the layout: nothing
// is read from the document it was parsed from, which is why a document built
// from scratch and one read from disk are written the same way.
//
// The spelling is the one on screen. Members are written in the order they
// are held, a key is followed by ": ", an empty container is written "{}" or
// "[]" on one line, and a string is quoted by QuoteString — all of which is
// what the JSON view draws, so that what is read is what will be saved.
//
// A number is written as the literal it was read as. 1.50 stays 1.50: pino
// shows a file rather than a rounded reading of one.
//
// Nothing in the tree is touched. Encoding a document neither rebuilds a node
// nor rewrites a slice, so it is safe to encode the version on screen while
// the user goes on editing the next one.
//
// A root that is not a node panics. A Node holding a nil pointer is a mistake
// in the caller, and a *Null one has no field to read, so it would otherwise
// be written as a legitimate JSON null: a value that went missing on the way
// here would reach the file as one the user appeared to have typed. Only the
// root is checked, since NewObject and NewArray keep a member or an element
// without a value out of every tree.
func Encode(n Node, f Format) []byte {
	checkFormat(f)

	if isNilNode(n) {
		panic("domain: cannot encode a node that is not there")
	}

	var b bytes.Buffer

	writeValue(&b, n, f, 0)

	if f.TrailingNL {
		b.WriteString(f.Newline)
	}

	return b.Bytes()
}

// checkFormat refuses a layout that would not produce a JSON document.
//
// It panics rather than returning an error, the way NewObject does for a
// member with no value: every production Format comes from DefaultFormat,
// DetectFormat or a command line flag that has already been checked, so an
// unusable one is a mistake in pino rather than something a document or a
// user can cause. The alternative is an error on Encode that every caller
// would have to handle and none could do anything about.
//
// Both fields are checked because both can silently produce something that is
// not JSON: a newline that is not one runs the members of an object together,
// and an indent holding anything but whitespace writes that text into the
// document.
func checkFormat(f Format) {
	if f.Newline != "\n" && f.Newline != "\r\n" {
		panic("domain: Format.Newline is " + strconv.Quote(f.Newline) + `, want "\n" or "\r\n"`)
	}

	for i := range len(f.Indent) {
		if c := f.Indent[i]; c != ' ' && c != '\t' {
			panic("domain: Format.Indent is " + strconv.Quote(f.Indent) + ", want whitespace")
		}
	}
}

// writeValue writes the subtree at n, which begins on a line already indented
// to depth.
//
// depth is how deep n sits rather than how far the line it starts on is
// indented: a value after a key starts where the key ended, and only the rows
// its own children open need a count. That is the same thing the renderer's
// Depth means, and keeping the two alike is what lets the layouts be compared
// by eye.
func writeValue(b *bytes.Buffer, n Node, f Format, depth int) {
	// Switching on Kind rather than on the concrete type so that a kind added
	// later is reported here by the exhaustive linter instead of falling
	// through to a panic at run time. domain sets the two together, so the
	// assertions below cannot fail.
	switch n.Kind() {
	case KindObject:
		writeObject(b, n.(*Object), f, depth)

	case KindArray:
		writeArray(b, n.(*Array), f, depth)

	case KindString:
		b.WriteString(QuoteString(n.(*String).Value()))

	case KindNumber:
		b.WriteString(n.(*Number).Raw())

	case KindBool:
		b.WriteString(strconv.FormatBool(n.(*Bool).Value()))

	case KindNull:
		b.WriteString("null")

	default:
		panic("domain: cannot encode node of kind " + n.Kind().String())
	}
}

// writeObject writes o, its braces included.
//
// An empty object is written on one line: the two rows a "{" and a "}" with
// nothing between them would take say no more than "{}" does, and the JSON
// view draws it that way for the same reason.
func writeObject(b *bytes.Buffer, o *Object, f Format, depth int) {
	if o.Len() == 0 {
		b.WriteString("{}")

		return
	}

	b.WriteByte('{')

	for i, m := range o.All() {
		if i > 0 {
			b.WriteByte(',')
		}

		writeLineStart(b, f, depth+1)
		b.WriteString(QuoteString(m.Key))
		b.WriteString(": ")
		writeValue(b, m.Value, f, depth+1)
	}

	writeLineStart(b, f, depth)
	b.WriteByte('}')
}

func writeArray(b *bytes.Buffer, a *Array, f Format, depth int) {
	if a.Len() == 0 {
		b.WriteString("[]")

		return
	}

	b.WriteByte('[')

	for i, e := range a.All() {
		if i > 0 {
			b.WriteByte(',')
		}

		writeLineStart(b, f, depth+1)
		writeValue(b, e, f, depth+1)
	}

	writeLineStart(b, f, depth)
	b.WriteByte(']')
}

// writeLineStart ends the line being written and indents the next one to
// depth.
//
// An empty indent still ends the line. Indentation is how wide a level is
// drawn, not whether a document has levels, and the JSON view shows one value
// per row whatever the width is: writing a document on a single line would be
// a different layout rather than a narrower one.
func writeLineStart(b *bytes.Buffer, f Format, depth int) {
	b.WriteString(f.Newline)

	for range depth {
		b.WriteString(f.Indent)
	}
}

// QuoteString wraps s in the double quotes of a JSON string, escaping what
// RFC 8259 requires and nothing else.
//
// It is used both when writing a document out and when showing a value on
// screen, so that what is displayed is what would be saved. strconv.Quote is
// not a substitute: it escapes to Go's rules, where a NUL comes out as \x00,
// which is not valid JSON.
//
// s must be valid UTF-8, which every string in a document is: NewString and
// NewObject refuse anything else, so the bytes that would make the result
// invalid JSON never reach a tree.
func QuoteString(s string) string {
	var b strings.Builder

	// Most strings need no escaping at all, so the quotes are the only
	// growth worth reserving.
	b.Grow(len(s) + 2)

	b.WriteByte('"')
	escapeInto(&b, s)
	b.WriteByte('"')

	return b.String()
}

// EscapeString is QuoteString without the surrounding quotes.
//
// It is what a row showing only the beginning of a long value needs: such a
// row closes with a marker inside the quotes, so the content and the quotes
// around it are written separately. Trimming the closing quote off
// QuoteString would do the same, at the cost of tying the caller to the shape
// of its result. Escaping is a rule of JSON, so the layer that owns the rule
// hands out both forms instead.
func EscapeString(s string) string {
	var b strings.Builder

	b.Grow(len(s))
	escapeInto(&b, s)

	return b.String()
}

// escapeInto writes s into b, escaped.
//
// Only " \ and the control characters below U+0020 have to be escaped. The
// solidus may be escaped and is not, since \/ is harder to read than / and
// both denote the same document. Everything else is written through as it
// stands, which keeps text in any language readable rather than turned into
// a run of \u escapes.
func escapeInto(b *strings.Builder, s string) {
	// Walking bytes rather than runes is safe: every byte of a multi-byte
	// UTF-8 sequence is 0x80 or above, so none of them can be mistaken for a
	// character that needs escaping, and they are copied through untouched.
	for i := range len(s) {
		switch c := s[i]; {
		case c == '"':
			b.WriteString(`\"`)
		case c == '\\':
			b.WriteString(`\\`)
		case c >= 0x20:
			b.WriteByte(c)
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\f':
			b.WriteString(`\f`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		default:
			// The remaining control characters have no short form.
			b.WriteString(`\u00`)
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0xf])
		}
	}
}

const hexDigits = "0123456789abcdef"
