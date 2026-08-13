package domain

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// How a string is spelled while someone is typing it, and how what they typed
// is read back.
//
// This is not the spelling the encoder uses. A document is written with the
// escapes RFC 8259 requires and no others (escapeInto), because that is what
// keeps a saved file close to the one that was read. What is typed has a
// second requirement: every character has to survive the trip to a terminal
// and back, and the ones that cannot be seen or typed have to have a form
// that can. So this escapes more, and the two rules are kept apart rather than
// one being bent to serve the other.

// InvalidEscapeError reports text that cannot be read back as a string.
//
// Reason is meant to be read by whoever typed the text, so it names the rule
// that was broken. Index is where in the text the trouble starts, which is
// what a caller pointing at it would need.
type InvalidEscapeError struct {
	Text   string
	Index  int
	Reason string
}

func (e *InvalidEscapeError) Error() string {
	return "invalid escape in " + strconv.Quote(e.Text) +
		" at " + strconv.Itoa(e.Index) + ": " + e.Reason
}

// EditableText is s as a person types it.
//
// It is the spelling the document itself uses — the same escapes a row on
// screen shows — with one departure: a line break is left as a line break,
// because a box being typed into has rows to draw one in and reading a value
// across them beats reading \n in the middle of it.
//
// Two more characters are escaped than a saved file would have: the control
// characters at U+007F and above, and U+FFFD. Neither is required by JSON.
// They are here because neither can be typed or told apart on a screen, and a
// value that cannot be typed is a value that cannot be edited without being
// damaged.
func EditableText(s string) string { return editable(s, true) }

// EditableLine is EditableText with the line breaks escaped as well, so that
// the whole of a value is one line however many breaks it holds.
func EditableLine(s string) string { return editable(s, false) }

func editable(s string, keepBreaks bool) string {
	var b strings.Builder

	// Most values need no escaping at all.
	b.Grow(len(s))

	for _, r := range s {
		switch {
		case r == '\n' && keepBreaks:
			b.WriteRune(r)

		case r == '"':
			b.WriteString(`\"`)

		case r == '\\':
			b.WriteString(`\\`)

		case r == '\b':
			b.WriteString(`\b`)

		case r == '\f':
			b.WriteString(`\f`)

		case r == '\n':
			b.WriteString(`\n`)

		case r == '\r':
			b.WriteString(`\r`)

		case r == '\t':
			b.WriteString(`\t`)

		// Every other control character, at U+007F and above included, and the
		// replacement character. The latter is escaped so that one the
		// document really holds can be told from one pino put on screen in
		// place of something it would not draw.
		case unicode.IsControl(r) || r == utf8.RuneError:
			b.WriteString(`\u`)
			b.WriteByte(hexDigits[(r>>12)&0xf])
			b.WriteByte(hexDigits[(r>>8)&0xf])
			b.WriteByte(hexDigits[(r>>4)&0xf])
			b.WriteByte(hexDigits[r&0xf])

		default:
			b.WriteRune(r)
		}
	}

	return b.String()
}

// ParseEditableText is the string that was typed, escapes undone.
//
// It reads back whatever EditableText and EditableLine write, which is what
// makes editing a value and committing it unchanged leave the document alone.
// It also accepts what they do not write: an escaped solidus, a line break
// where EditableLine would have put \n, and a character spelled \u that had no
// need to be.
//
// Bytes are walked rather than runes. Every byte of a multi-byte sequence is
// 0x80 or above, so none can be mistaken for a backslash, and copying them
// through keeps text in any language exactly as it was typed.
func ParseEditableText(s string) (string, error) {
	// Nothing to undo, which is the common case: a value with no escape in it
	// is itself.
	if !strings.ContainsRune(s, '\\') {
		return s, nil
	}

	var b strings.Builder

	b.Grow(len(s))

	for i := 0; i < len(s); {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			i++

			continue
		}

		r, next, err := unescapeAt(s, i)
		if err != nil {
			return "", err
		}

		b.WriteRune(r)
		i = next
	}

	return b.String(), nil
}

// unescapeAt reads the escape beginning at the backslash at i, and answers
// with the character it stands for and where the text goes on.
func unescapeAt(s string, i int) (rune, int, error) {
	fail := func(at int, reason string) (rune, int, error) {
		return 0, 0, &InvalidEscapeError{Text: s, Index: at, Reason: reason}
	}

	if i+1 >= len(s) {
		return fail(i, "a backslash with nothing after it")
	}

	switch c := s[i+1]; c {
	case '"', '\\', '/':
		return rune(c), i + 2, nil

	case 'b':
		return '\b', i + 2, nil

	case 'f':
		return '\f', i + 2, nil

	case 'n':
		return '\n', i + 2, nil

	case 'r':
		return '\r', i + 2, nil

	case 't':
		return '\t', i + 2, nil

	case 'u':
		return unescapeCodepoint(s, i)

	default:
		return fail(i, `\`+string(rune(c))+" is not an escape")
	}
}

// unescapeCodepoint reads the \u escape at i, and the one after it when the
// first is half of a surrogate pair.
//
// A pair is one character written as two escapes, which is how JSON spells
// anything above U+FFFF. Half of one on its own is not a character and cannot
// be put in a Go string, so it is refused here rather than becoming a
// replacement character nobody asked for.
func unescapeCodepoint(s string, i int) (rune, int, error) {
	fail := func(at int, reason string) (rune, int, error) {
		return 0, 0, &InvalidEscapeError{Text: s, Index: at, Reason: reason}
	}

	first, ok := hexQuad(s, i)
	if !ok {
		return fail(i, `\u needs four hexadecimal digits`)
	}

	if !utf16.IsSurrogate(first) {
		return first, i + 6, nil
	}

	// The other half has to be the very next escape.
	if i+6 < len(s) && s[i+6] == '\\' && i+7 < len(s) && s[i+7] == 'u' {
		if second, ok := hexQuad(s, i+6); ok {
			if paired := utf16.DecodeRune(first, second); paired != utf8.RuneError {
				return paired, i + 12, nil
			}
		}
	}

	return fail(i, "half of a character, with no other half after it")
}

// hexQuad reads the four hexadecimal digits of the \u escape at i.
func hexQuad(s string, i int) (rune, bool) {
	if i+6 > len(s) {
		return 0, false
	}

	v, err := strconv.ParseUint(s[i+2:i+6], 16, 32)
	if err != nil {
		return 0, false
	}

	return rune(v), true
}
