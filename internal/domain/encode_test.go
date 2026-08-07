package domain

import "testing"

func TestQuoteString(t *testing.T) {
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
