package documentview

import (
	"slices"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

func TestLineKindSelectsOnlyNodeRows(t *testing.T) {
	t.Parallel()

	tests := map[LineKind]bool{
		LineSingle:  true,
		LineOpen:    true,
		LineClose:   false,
		LineComment: false,
	}

	for kind, want := range tests {
		if got := kind.Selectable(); got != want {
			t.Errorf("%s.Selectable() = %t, want %t", kind, got, want)
		}
	}
}

func TestBlockCommentsBecomePhysicalRows(t *testing.T) {
	t.Parallel()

	comment, err := domain.NewComment(" first\r\n second\r third\n", true, true)
	if err != nil {
		t.Fatalf("NewComment: %v", err)
	}

	lines := appendComments(nil, func(yield func(domain.Comment) bool) { yield(comment) }, domain.Path{}, 2)
	want := []string{"/* first", " second", " third", "*/"}
	if len(lines) != len(want) {
		t.Fatalf("comment row count = %d, want %d", len(lines), len(want))
	}

	if got := []string{lines[0].Text(), lines[1].Text(), lines[2].Text(), lines[3].Text()}; !slices.Equal(got, want) {
		t.Errorf("comment rows = %q, want %q", got, want)
	}
	if got := []int{lines[0].Depth, lines[1].Depth, lines[2].Depth, lines[3].Depth}; !slices.Equal(got, []int{2, 0, 0, 0}) {
		t.Errorf("comment depths = %v, want [2 0 0 0]", got)
	}
}

func TestCommentsBeforeANodeKeepTheirOrder(t *testing.T) {
	t.Parallel()

	inline := commentForRenderTest(t, " first ", true, false)
	ownLine := commentForRenderTest(t, " second", false, true)
	lastInline := commentForRenderTest(t, " third ", true, false)

	rows, spans := commentsBefore(
		func(yield func(domain.Comment) bool) {
			yield(inline)
			yield(ownLine)
			yield(lastInline)
		},
		domain.Path{}, 1,
	)
	if len(rows) != 2 {
		t.Fatalf("comment row count = %d, want 2", len(rows))
	}

	if got, want := []string{rows[0].Text(), rows[1].Text()}, []string{"/* first */", "// second"}; !slices.Equal(got, want) {
		t.Errorf("comment rows = %q, want %q", got, want)
	}
	if got, want := spanText(spans), "/* third */ "; got != want {
		t.Errorf("inline spans = %q, want %q", got, want)
	}
}

func commentForRenderTest(t *testing.T, text string, block, ownLine bool) domain.Comment {
	t.Helper()

	comment, err := domain.NewComment(text, block, ownLine)
	if err != nil {
		t.Fatalf("NewComment: %v", err)
	}

	return comment
}

func spanText(spans []Span) string {
	var text string
	for _, span := range spans {
		text += span.Text
	}

	return text
}

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
