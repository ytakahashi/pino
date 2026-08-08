package jsonparser

import (
	"errors"
	"testing"
)

func TestLineColumn(t *testing.T) {
	src := []byte("ab\ncde\n\nf")

	tests := []struct {
		name         string
		offset       int
		line, column int
	}{
		{"start", 0, 1, 1},
		{"within the first line", 1, 1, 2},
		{"the first newline", 2, 1, 3},
		{"start of the second line", 3, 2, 1},
		{"within the second line", 5, 2, 3},
		{"an empty line", 7, 3, 1},
		{"the last line", 8, 4, 1},
		{"end of input", 9, 4, 2},
		{"past the end is clamped", 99, 4, 2},
		{"before the start is clamped", -1, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, column := lineColumn(src, tt.offset)
			if line != tt.line || column != tt.column {
				t.Errorf("lineColumn(%d) = %d:%d, want %d:%d", tt.offset, line, column, tt.line, tt.column)
			}
		})
	}
}

// TestLineColumnCountsBytes fixes the choice of unit. The underlying parser
// counts bytes, and a position pino computes itself has to agree with one it
// reports, so neither may count characters.
func TestLineColumnCountsBytes(t *testing.T) {
	src := []byte(`"鍵"`)

	if _, column := lineColumn(src, 4); column != 5 {
		t.Errorf("column = %d, want 5", column)
	}
}

// TestParseHujsonError covers taking apart the one string the library reports
// a failure in.
func TestParseHujsonError(t *testing.T) {
	tests := []struct {
		name         string
		err          string
		line, column int
		msg          string
	}{
		{
			name: "the shape the library uses",
			err:  "hujson: line 3, column 5: invalid character '}' at start of value",
			line: 3, column: 5,
			msg: "invalid character '}' at start of value",
		},
		{
			name: "a wrapped reason",
			err:  "hujson: line 1, column 1: parsing value: unexpected EOF",
			line: 1, column: 1,
			msg: "parsing value: unexpected EOF",
		},
		{
			name: "a reason spanning lines",
			err:  "hujson: line 2, column 4: something\nover two lines",
			line: 2, column: 4,
			msg: "something\nover two lines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHujsonError(errors.New(tt.err))

			if got.Line != tt.line || got.Column != tt.column {
				t.Errorf("position = %d:%d, want %d:%d", got.Line, got.Column, tt.line, tt.column)
			}

			if got.Msg != tt.msg {
				t.Errorf("Msg = %q, want %q", got.Msg, tt.msg)
			}
		})
	}
}

// TestParseHujsonErrorFallback is the reason the shape is matched rather than
// assumed. The library gives no typed error and could reword or restructure
// its messages; pino then loses the position, and must not lose the message
// with it or fail trying to read one that is not there.
func TestParseHujsonErrorFallback(t *testing.T) {
	tests := []struct {
		name string
		err  string
	}{
		{"no prefix at all", "something else entirely"},
		{"the prefix without a position", "hujson: invalid character"},
		{"a reworded position", "hujson: at line 3, column 5: oops"},
		{"a non-numeric line", "hujson: line X, column 5: oops"},
		{"a line number too large to hold", "hujson: line 99999999999999999999, column 5: oops"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHujsonError(errors.New(tt.err))

			if got.Line != 0 || got.Column != 0 {
				t.Errorf("position = %d:%d, want it to be unknown", got.Line, got.Column)
			}

			if got.Msg != tt.err {
				t.Errorf("Msg = %q, want the whole message %q", got.Msg, tt.err)
			}
		})
	}
}

func TestSyntaxErrorError(t *testing.T) {
	tests := []struct {
		name string
		err  *SyntaxError
		want string
	}{
		{"with a position", &SyntaxError{Line: 3, Column: 5, Msg: "oops"}, "3:5: oops"},
		{"without a position", &SyntaxError{Msg: "oops"}, "oops"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSyntaxErrorUnwrap(t *testing.T) {
	cause := errors.New("cause")
	err := &SyntaxError{Msg: "oops", Err: cause}

	if !errors.Is(err, cause) {
		t.Error("errors.Is does not find the cause")
	}

	if errors.Unwrap(&SyntaxError{Msg: "oops"}) != nil {
		t.Error("an error with no cause unwraps to something")
	}
}
