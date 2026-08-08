package domain

import "strings"

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
