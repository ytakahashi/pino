package documentview

import (
	"testing"
)

func TestStringSpanShortensAndEscapesAValue(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value  string
		maxLen int
		want   string
	}{
		"shorter than the limit": {value: "abc", maxLen: 5, want: `"abc"`},
		"exactly the limit":      {value: "abcde", maxLen: 5, want: `"abcde"`},
		"one rune over":          {value: "abcdef", maxLen: 5, want: `"abcde…"`},
		"empty":                  {value: "", maxLen: 5, want: `""`},

		// No limit at all: the value is drawn however long it is, which is
		// what the golden files of the other tests rely on.
		"no limit":       {value: "abcdef", maxLen: 0, want: `"abcdef"`},
		"negative limit": {value: "abcdef", maxLen: -1, want: `"abcdef"`},

		// The limit counts runes, so a multi-byte character is one of them and
		// is never cut in half.
		"kanji under the limit": {value: "設定", maxLen: 5, want: `"設定"`},
		"kanji over the limit":  {value: "設定ファイル", maxLen: 3, want: `"設定フ…"`},
		"an emoji":              {value: "🌲🌲🌲", maxLen: 2, want: `"🌲🌲…"`},

		// A cut that lands next to text needing an escape. Escaping after the
		// cut is what keeps the sequence whole; cutting the escaped form could
		// leave "\u00" on screen.
		"cut before an escape": {value: "ab\ncd", maxLen: 2, want: `"ab…"`},
		"cut after an escape":  {value: "ab\ncd", maxLen: 3, want: `"ab\n…"`},
		"cut before a quote":   {value: `ab"cd`, maxLen: 2, want: `"ab…"`},
		"cut after a quote":    {value: `ab"cd`, maxLen: 3, want: `"ab\"…"`},
		"cut before a nul":     {value: "ab\x00cd", maxLen: 3, want: `"ab\u0000…"`},

		// A limit of one is still a limit, not a request for everything.
		"limit of one": {value: "abc", maxLen: 1, want: `"a…"`},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := stringSpan(tc.value, tc.maxLen)

			if got.Text != tc.want {
				t.Errorf("stringSpan(%q, %d) = %s, want %s", tc.value, tc.maxLen, got.Text, tc.want)
			}

			if got.Role != RoleStringValue {
				t.Errorf("Role = %v, want %v; shortening does not make a value something else",
					got.Role, RoleStringValue)
			}
		})
	}
}

// A value short enough to be drawn in full has to come out exactly as it would
// without a limit set, or the limit would be changing documents it does not
// shorten.
func TestStringSpanLeavesShortValuesAlone(t *testing.T) {
	t.Parallel()

	values := []string{"", "a", "abcde", "設定", `say "hi"`, "tab\there", "nul:\x00"}

	for _, v := range values {
		t.Run(v, func(t *testing.T) {
			t.Parallel()

			want := stringSpan(v, 0)
			if got := stringSpan(v, 64); got != want {
				t.Errorf("stringSpan(%q, 64) = %s, want %s", v, got.Text, want.Text)
			}
		})
	}
}
