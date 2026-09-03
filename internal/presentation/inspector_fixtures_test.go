package presentation

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
	"github.com/ytakahashi/pino/internal/application/documentview"
)

// stripped is a pane read back without styling, one entry per row, every row
// still filled out to the width the pane was asked for.
func stripped(pane []string) []string {
	rows := make([]string, 0, len(pane))
	for _, row := range pane {
		rows = append(rows, ansi.Strip(row))
	}

	return rows
}

// plain is stripped with the filling dropped, for the tests whose rows carry
// nothing meaningful at their right hand end.
func plain(pane []string) []string {
	rows := stripped(pane)
	for i, row := range rows {
		rows[i] = strings.TrimRight(row, " ")
	}

	return rows
}

// scalarInfo and containerInfo stand for the two shapes the pane describes.
func scalarInfo() application.InspectorInfo {
	return application.InspectorInfo{
		Pointer: "/server/port",
		Type:    "number",
		Value:   documentview.Span{Text: "8080", Role: documentview.RoleNumberValue},
		Label:   "port",
		Naming:  application.NamedKey,
	}
}

func containerInfo() application.InspectorInfo {
	return application.InspectorInfo{
		Pointer:   "/server",
		Type:      "object",
		Container: true,
		Children:  3,
		Foldable:  true,
		Label:     "server",
		Naming:    application.NamedKey,
	}
}

func longValueInfo() application.InspectorInfo {
	info := scalarInfo()
	info.Value = documentview.Span{
		Text: `"` + strings.Repeat("long ", 40) + `"`,
		Role: documentview.RoleStringValue,
	}

	return info
}
