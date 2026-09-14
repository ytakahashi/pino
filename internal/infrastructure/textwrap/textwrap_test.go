package textwrap

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestWrapLineFitsEveryRowAndLosesNothing(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		text  string
		width int
	}{
		"ascii":             {text: "the quick brown fox jumps over the lazy dog", width: 10},
		"spaces at the cut": {text: "aaaa    bbbb    cccc", width: 6},
		"japanese":          {text: "設定ファイルに日本語が入るのは普通のことである", width: 9},
		"emoji":             {text: "ok 👍🏽 done 🎉🎉🎉 end", width: 5},
		"combining":         {text: "café café café", width: 4},
		"mixed":             {text: `"note: 日本語 and ascii 😀 together"`, width: 7},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rows, truncated := New().WrapLine(tt.text, tt.width, len(tt.text)+1)
			if truncated {
				t.Fatal("WrapLine reports truncating an unbounded result")
			}

			for i, row := range rows {
				if w := ansi.StringWidth(row); w > tt.width {
					t.Errorf("row %d %q is %d columns, want at most %d", i, row, w, tt.width)
				}
			}

			if got := strings.Join(rows, ""); got != tt.text {
				t.Errorf("joined rows = %q, want %q", got, tt.text)
			}
		})
	}
}

func TestWrapLineKeepsTextThatFitsOnOneRow(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		text  string
		width int
	}{
		"empty":       {text: "", width: 10},
		"exact width": {text: "日本語", width: 6},
		"shorter":     {text: "abc", width: 10},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, truncated := New().WrapLine(tt.text, tt.width, 1)
			if want := []string{tt.text}; !slices.Equal(got, want) || truncated {
				t.Errorf("WrapLine(%q, %d) = %q, want %q", tt.text, tt.width, got, want)
			}
		})
	}
}

func TestWrapLineLeavesTextWholeBelowTwoColumns(t *testing.T) {
	t.Parallel()

	const text = "日本語 text"

	for _, width := range []int{-1, 0, 1} {
		got, truncated := New().WrapLine(text, width, 1)
		if want := []string{text}; !slices.Equal(got, want) || truncated {
			t.Errorf("WrapLine(%q, %d) = %q, want %q", text, width, got, want)
		}
	}
}

// Truncation is reported exactly when another row would be needed: text that
// fills the last allowed row to its end is whole, and one character more is
// not. What is returned either way fits the width and begins the text.
func TestWrapLineStopsAtTheRowLimit(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		text           string
		width, maxRows int
		want           []string
		truncated      bool
	}{
		"far beyond the limit": {
			text: strings.Repeat("0123456789", 10_000), width: 10, maxRows: 2,
			want: []string{"0123456789", "0123456789"}, truncated: true,
		},
		"one character over": {
			text: "01234567890", width: 5, maxRows: 2,
			want: []string{"01234", "56789"}, truncated: true,
		},
		"exactly the limit": {
			text: "0123456789", width: 5, maxRows: 2,
			want: []string{"01234", "56789"}, truncated: false,
		},
		"wide characters over": {
			text: "日本語日本語日本語", width: 5, maxRows: 2,
			want: []string{"日本", "語日"}, truncated: true,
		},
		"wide characters exactly": {
			text: "日本日本", width: 4, maxRows: 2,
			want: []string{"日本", "日本"}, truncated: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rows, truncated := New().WrapLine(tt.text, tt.width, tt.maxRows)

			if !slices.Equal(rows, tt.want) || truncated != tt.truncated {
				t.Errorf("WrapLine() = %q, %t, want %q, %t", rows, truncated, tt.want, tt.truncated)
			}

			for i, row := range rows {
				if w := ansi.StringWidth(row); w > tt.width {
					t.Errorf("row %d %q is %d columns, want at most %d", i, row, w, tt.width)
				}
			}

			if joined := strings.Join(rows, ""); !strings.HasPrefix(tt.text, joined) || (joined == tt.text) == truncated {
				t.Errorf("joined rows = %q with truncated %t, want the whole text exactly when not truncated", joined, truncated)
			}
		})
	}
}

func TestWrapLineWithNoRowsReportsTextLeftOut(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"", "value"} {
		rows, truncated := New().WrapLine(text, 10, 0)
		if len(rows) != 0 || truncated != (text != "") {
			t.Errorf("WrapLine(%q, 10, 0) = %q, %t", text, rows, truncated)
		}
	}
}
