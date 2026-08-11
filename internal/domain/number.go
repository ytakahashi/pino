package domain

import "strconv"

// InvalidNumberError reports text that is not a JSON number.
//
// Reason is meant to be read by whoever typed the text, so it names the rule
// that was broken where one can be named. It is not exhaustive: text that goes
// wrong in no particular way is simply not a number.
type InvalidNumberError struct {
	Text   string
	Reason string
}

func (e *InvalidNumberError) Error() string {
	return "invalid JSON number " + strconv.Quote(e.Text) + ": " + e.Reason
}

// ParseNumber builds a Number from text typed by a person.
//
// The grammar is RFC 8259's and nothing more: no leading plus, no leading
// zero, no hexadecimal, no Infinity, and no surrounding space. What a JSON
// number may look like is knowledge about the format, so it lives beside
// QuoteString rather than in whichever layer happens to read a keystroke.
//
// NewNumber is left unchecked because the parser hands it literals hujson has
// already accepted, and checking them again would re-scan every number in a
// document that is only being read. The invariant is instead kept by every
// route into the tree checking its own input, and this is the other route.
//
// The text is kept exactly as it was given, so a number entered as 1.50 is
// saved as 1.50. Surrounding space is refused rather than trimmed, for the
// same reason: what is on screen is what would be written out.
func ParseNumber(text string) (*Number, error) {
	if err := checkNumber(text); err != nil {
		return nil, err
	}

	return NewNumber(text), nil
}

// checkNumber reports whether text is a JSON number, and why not when it is
// not.
//
// The grammar is scanned by hand rather than handed to strconv.ParseFloat,
// which accepts "+1", "0x10", "Inf" and surrounding space, and which would
// answer with a float64 that has already lost the precision and the notation
// Number exists to preserve.
func checkNumber(text string) error {
	fail := func(reason string) error {
		return &InvalidNumberError{Text: text, Reason: reason}
	}

	if text == "" {
		return fail("a number cannot be empty")
	}

	i := 0

	// The sign. A leading plus is called out because it is the one form a
	// person is likely to type on purpose.
	switch text[i] {
	case '+':
		return fail("a leading plus is not allowed")
	case '-':
		i++
	}

	// The integer part: a single zero, or a digit string not starting with
	// one.
	switch {
	case i >= len(text):
		return fail("a digit must follow the minus sign")

	case text[i] == '0':
		i++
		if i < len(text) && isDigit(text[i]) {
			return fail("leading zeros are not allowed")
		}

	case isDigit(text[i]):
		i = skipDigits(text, i)

	default:
		return fail("not a JSON number")
	}

	// The fraction.
	if i < len(text) && text[i] == '.' {
		i++
		if i >= len(text) || !isDigit(text[i]) {
			return fail("a digit must follow the decimal point")
		}

		i = skipDigits(text, i)
	}

	// The exponent.
	if i < len(text) && (text[i] == 'e' || text[i] == 'E') {
		i++
		if i < len(text) && (text[i] == '+' || text[i] == '-') {
			i++
		}

		if i >= len(text) || !isDigit(text[i]) {
			return fail("a digit must follow the exponent")
		}

		i = skipDigits(text, i)
	}

	// Anything left over — trailing space, a comma carried over from a copied
	// line, the "x10" of a hexadecimal literal.
	if i != len(text) {
		return fail("not a JSON number")
	}

	return nil
}

// isDigit reports whether c is one of the ten ASCII digits.
//
// Bytes are compared rather than runes, which is what refuses the full-width
// digits an input method can produce: they are not the digits JSON is written
// with, however much they look like them.
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func skipDigits(text string, i int) int {
	for i < len(text) && isDigit(text[i]) {
		i++
	}

	return i
}
