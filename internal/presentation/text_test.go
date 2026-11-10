package presentation

import (
	"testing"
	"unicode/utf8"
)

func TestEscapedKeepsNamesApartOnOneRow(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		in   string
		want string
	}{
		"plain text":         {in: "config.json", want: "config.json"},
		"wide characters":    {in: "設定.json", want: "設定.json"},
		"newline and tab":    {in: "a\nb\tc", want: `a\nb\tc`},
		"escape sequence":    {in: "\x1b[31mred", want: `\x1b[31mred`},
		"bidi override":      {in: "a" + string(rune(0x202e)) + "b", want: `a\u202eb`},
		"invalid UTF-8":      {in: "a\xffb", want: `a\xffb`},
		"replacement rune":   {in: "a" + string(utf8.RuneError) + "b", want: "a" + string(utf8.RuneError) + "b"},
		"quotes stay":        {in: `a"b'c`, want: `a"b'c`},
		"backslash stays":    {in: `a\b`, want: `a\b`},
		"a quoted character": {in: `invalid character '"' after object key`, want: `invalid character '"' after object key`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := escaped(tc.in); got != tc.want {
				t.Errorf("escaped(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
