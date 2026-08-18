package presentation

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
)

// statusText is the bar as it is drawn by default, with mouse reporting on.
func statusText(theme Theme, info application.StatusInfo, lines int, pending Pending, width int) string {
	state := barState{Lines: lines, Pending: pending, Mouse: true}

	return statusTextForState(theme, info, state, width)
}

// statusTextForState is the bar without styling, with the run of spaces
// between its two ends collapsed to the separator that divides the fields.
// What is left is the fields in the order they are drawn.
func statusTextForState(theme Theme, info application.StatusInfo, state barState, width int) string {
	bar := ansi.Strip(theme.RenderStatusBar(info, state, width))

	return strings.TrimRight(gapPattern.ReplaceAllString(bar, separator), " ")
}

// gapPattern matches the space holding the two ends of the bar apart.
var gapPattern = regexp.MustCompile(` {3,}`)

func withDirty(info application.StatusInfo) application.StatusInfo {
	info.Dirty = true

	return info
}

func withNew(info application.StatusInfo) application.StatusInfo {
	info.New = true

	return info
}

func withNotice(info application.StatusInfo, summary string) application.StatusInfo {
	info.Notice = &application.NoticeInfo{Summary: summary}

	return info
}

func withIndent(info application.StatusInfo, indent string) application.StatusInfo {
	info.Indent = indent

	return info
}

func withCursor(info application.StatusInfo, pointer, typ string) application.StatusInfo {
	info.Pointer, info.Type = pointer, typ

	return info
}

func withView(info application.StatusInfo, view application.ViewMode) application.StatusInfo {
	info.ViewMode = view

	return info
}
