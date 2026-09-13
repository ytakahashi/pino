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

			rows := New().WrapLine(tt.text, tt.width)

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

			if got, want := New().WrapLine(tt.text, tt.width), []string{tt.text}; !slices.Equal(got, want) {
				t.Errorf("WrapLine(%q, %d) = %q, want %q", tt.text, tt.width, got, want)
			}
		})
	}
}

func TestWrapLineLeavesTextWholeBelowTwoColumns(t *testing.T) {
	t.Parallel()

	const text = "日本語 text"

	for _, width := range []int{-1, 0, 1} {
		if got, want := New().WrapLine(text, width), []string{text}; !slices.Equal(got, want) {
			t.Errorf("WrapLine(%q, %d) = %q, want %q", text, width, got, want)
		}
	}
}
