package presentation

import (
	"strings"
	"unicode"
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
