package jsonparser

import (
	"bytes"
	"regexp"
	"strconv"
)

// SyntaxError is a reason a document could not be read, reported against a
// position in its source.
//
// Everything that stops a document from being opened is one of these: a
// malformed literal, a construct the dialect does not accept, and text that
// breaks an invariant of the tree. They differ in what has to be said about
// them, not in what the caller does with them, which is to print one line
// naming a position. Err keeps the distinction available to whoever wants it.
type SyntaxError struct {
	// Line and Column are 1-based, and count bytes rather than characters, as
	// the underlying parser does. Both are 0 when the position is unknown.
	Line   int
	Column int

	Msg string

	// Err is the cause when there is a typed one, so that a caller can pick
	// out for instance a *domain.DuplicateKeyError with errors.As.
	Err error
}

func (e *SyntaxError) Error() string {
	if e.Line == 0 {
		return e.Msg
	}

	return strconv.Itoa(e.Line) + ":" + strconv.Itoa(e.Column) + ": " + e.Msg
}

func (e *SyntaxError) Unwrap() error { return e.Err }

// errorAt reports msg against a byte offset in src.
func errorAt(src []byte, offset int, msg string, cause error) *SyntaxError {
	line, column := lineColumn(src, offset)

	return &SyntaxError{Line: line, Column: column, Msg: msg, Err: cause}
}

// lineColumn converts a byte offset in src into a 1-based line and column.
//
// The underlying parser reports the positions it finds this way but does not
// export the conversion, so positions pino works out itself are computed the
// same way in order to agree with them.
func lineColumn(src []byte, offset int) (line, column int) {
	offset = min(max(offset, 0), len(src))

	line = 1 + bytes.Count(src[:offset], []byte("\n"))
	column = 1 + offset - (bytes.LastIndexByte(src[:offset], '\n') + 1)

	return line, column
}

// hujsonMessage matches the single string form the underlying parser reports
// a failure in. The (?s) lets the reason span lines.
var hujsonMessage = regexp.MustCompile(`^hujson: line (\d+), column (\d+): (?s)(.*)$`)

// parseHujsonError recovers the position and the reason from a parse failure.
//
// The library formats both into the error string and offers nothing to read
// them back from, so they are taken apart again here. A message that does not
// fit the shape is kept whole with no position, rather than dropped or guessed
// at: pino has to survive the library rewording its errors, and a wrong
// position would be worse than none.
func parseHujsonError(err error) *SyntaxError {
	m := hujsonMessage.FindStringSubmatch(err.Error())
	if m == nil {
		return &SyntaxError{Msg: err.Error(), Err: err}
	}

	line, lineErr := strconv.Atoi(m[1])
	column, columnErr := strconv.Atoi(m[2])
	if lineErr != nil || columnErr != nil {
		return &SyntaxError{Msg: err.Error(), Err: err}
	}

	return &SyntaxError{Line: line, Column: column, Msg: m[3], Err: err}
}
