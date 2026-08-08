package presentation

import (
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
)

// statusText is the bar without styling or trailing padding, which is what
// the fields put in it can be checked against.
func statusText(theme Theme, info application.StatusInfo, lines, width int) string {
	return strings.TrimRight(ansi.Strip(theme.RenderStatusBar(info, lines, width)), " ")
}

func TestRenderStatusBarFields(t *testing.T) {
	open := application.StatusInfo{
		Mode:     application.ModeNormal,
		ViewMode: application.ViewJSON,
		Name:     "config.json",
		Indent:   "  ",
	}

	tests := []struct {
		name  string
		info  application.StatusInfo
		lines int
		want  string
	}{
		{
			name:  "open document",
			info:  open,
			lines: 11,
			want:  " NORMAL  JSON  config.json  11 lines  indent:2",
		},
		{
			name:  "single line",
			info:  open,
			lines: 1,
			want:  " NORMAL  JSON  config.json  1 line  indent:2",
		},
		{
			// A document nothing has been read into still has a mode and a
			// view, and the bar says so rather than going blank.
			name:  "nothing open",
			info:  application.StatusInfo{Indent: "  "},
			lines: 0,
			want:  " NORMAL  JSON  0 lines  indent:2",
		},
		{
			name:  "unsaved changes",
			info:  withDirty(open),
			lines: 11,
			want:  " NORMAL  JSON  config.json  11 lines  indent:2  modified",
		},
		{
			name:  "tabs",
			info:  withIndent(open, "\t"),
			lines: 11,
			want:  " NORMAL  JSON  config.json  11 lines  indent:tab",
		},
		{
			name:  "four spaces",
			info:  withIndent(open, "    "),
			lines: 11,
			want:  " NORMAL  JSON  config.json  11 lines  indent:4",
		},
		{
			// Only an explicit choice produces this: a document detected as
			// having no indentation falls back to the default instead.
			name:  "no indentation",
			info:  withIndent(open, ""),
			lines: 11,
			want:  " NORMAL  JSON  config.json  11 lines  indent:none",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusText(Theme{}, tc.info, tc.lines, 80); got != tc.want {
				t.Errorf("RenderStatusBar() = %q, want %q", got, tc.want)
			}
		})
	}
}

// The bar occupies its width exactly, in one row. Anything wider would wrap
// and push the document off the top of the screen; anything narrower would
// leave the strip unfinished.
func TestRenderStatusBarOccupiesExactlyOneRowOfTheWidth(t *testing.T) {
	info := application.StatusInfo{
		Mode:     application.ModeNormal,
		ViewMode: application.ViewJSON,
		Name:     "a-rather-long-file-name.json",
		Indent:   "  ",
	}

	for _, width := range []int{120, 45, 12, 1} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			got := DefaultTheme().RenderStatusBar(info, 11, width)

			if strings.Contains(got, "\n") {
				t.Fatalf("RenderStatusBar() spans several rows: %q", got)
			}

			if w := lipgloss.Width(got); w != width {
				t.Errorf("RenderStatusBar() width = %d, want %d", w, width)
			}
		})
	}
}

// A file name is the one piece of text on the bar that comes from outside
// pino, and on Unix it may hold anything but a slash and a NUL. Neither a
// newline nor an escape sequence in one may reach the terminal: the first
// would break the bar across rows, the second would be carried out.
func TestRenderStatusBarNeutralisesTheFileName(t *testing.T) {
	tests := []struct {
		name string
		file string
		want string
	}{
		{name: "newline", file: "two\nlines.json", want: "two�lines.json"},
		{name: "carriage return", file: "back\rup.json", want: "back�up.json"},
		{name: "tab", file: "a\tb.json", want: "a�b.json"},
		{name: "delete", file: "a\x7fb.json", want: "a�b.json"},
		{name: "escape sequence", file: "\x1b[31mred\x1b[m.json", want: "�[31mred�[m.json"},
		{name: "invalid utf-8", file: "a\xffb.json", want: "a�b.json"},

		// Nothing is done to a name that has nothing wrong with it, wide
		// characters and all.
		{name: "left alone", file: "設定.json", want: "設定.json"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := application.StatusInfo{Name: tc.file, Indent: "  "}

			// The zero theme adds no escapes of its own, so what is left in
			// the result came from the name.
			got := Theme{}.RenderStatusBar(info, 11, 80)

			if strings.ContainsRune(got, '\n') {
				t.Errorf("RenderStatusBar() spans several rows: %q", got)
			}

			if strings.ContainsRune(got, '\x1b') {
				t.Errorf("RenderStatusBar() passed an escape through: %q", got)
			}

			if !strings.Contains(got, tc.want) {
				t.Errorf("RenderStatusBar() = %q, want it to hold %q", got, tc.want)
			}
		})
	}
}

func withDirty(info application.StatusInfo) application.StatusInfo {
	info.Dirty = true

	return info
}

func withIndent(info application.StatusInfo, indent string) application.StatusInfo {
	info.Indent = indent

	return info
}
