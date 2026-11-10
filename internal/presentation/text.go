package presentation

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file holds the handling of text that came from outside pino and is
// drawn as it stands. The status bar and the inspector both show such text,
// and both have to show it the same way.

// pointerLabel is how a JSON Pointer is shown.
//
// RFC 6901 spells the root as the empty string, which on screen reads as
// nothing being selected. The document's own root is written the way a path to
// it would be.
func pointerLabel(pointer string) string {
	if pointer == "" {
		return "/"
	}

	return pointer
}

// printable is outside text made safe to draw.
//
// Text carrying a newline would break a row in two and text carrying an escape
// sequence would be obeyed by the terminal rather than shown. Three of the
// things on screen come from outside and are not quoted on the way: the name
// of the file, the pointer to the selected node, whose tokens are the
// document's own keys with only "~" and "/" escaped, and the name that node
// has within its parent.
//
// Each offending rune becomes a replacement character, which keeps the width
// on screen the count of what it holds. Bytes that are not valid UTF-8 come
// out the same way, since decoding them yields that rune already.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return unicode.ReplacementChar
		}

		return r
	}, s)
}

// escaped is outside text made safe to draw while keeping it distinguishable.
//
// The picker lists names that only differ in what printable would replace:
// "a\nb.json" and "a\tb.json" are two files, and a reader choosing between
// them has to see which is which. Each rune that is not graphic is written as
// its Go escape instead, and a byte that is not valid UTF-8 as \xNN. Quotes
// and backslashes stay as they are, unlike strconv.Quote: they are ordinary
// in file names and in parse errors such as "invalid character '"'". The cost
// is that a name holding a literal backslash followed by "n" reads the same as
// one holding a newline, which is rare enough to accept.
//
// The editor keeps printable, since it shows one name and never has to tell
// two apart.
func escaped(s string) string {
	var b strings.Builder

	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])

		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])

		case unicode.IsGraphic(r):
			b.WriteRune(r)

		default:
			quoted := strconv.QuoteRune(r)
			b.WriteString(quoted[1 : len(quoted)-1])
		}

		i += size
	}

	return b.String()
}
