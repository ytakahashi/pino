package presentation

import (
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
)

// bandRows is the band without styling, one entry per row.
func bandRows(p application.PromptInfo, input []string, width int) []string {
	rows := DefaultTheme().RenderPrompt(p, input, width)

	stripped := make([]string, 0, len(rows))
	for _, row := range rows {
		stripped = append(stripped, ansi.Strip(row))
	}

	return stripped
}

// typePrompt is the list of types, as the session offers it.
func typePrompt() application.PromptInfo {
	return application.PromptInfo{
		Kind:  application.PromptChoice,
		Title: "type",
		Choices: []application.Choice{
			{Key: 's', Label: "string"},
			{Key: 'n', Label: "number"},
			{Key: 'b', Label: "boolean"},
			{Key: 'z', Label: "null"},
			{Key: 'o', Label: "object"},
			{Key: 'a', Label: "array"},
		},
	}
}

func confirmPrompt() application.PromptInfo {
	return application.PromptInfo{
		Kind:    application.PromptChoice,
		Title:   "Discard 12 child nodes under /server?",
		Choices: []application.Choice{{Key: 'y', Label: "Yes"}, {Key: 'n', Label: "No"}},
	}
}

func textPrompt(multiline bool) application.PromptInfo {
	return application.PromptInfo{
		Kind:      application.PromptText,
		Title:     "Edit number",
		Multiline: multiline,
	}
}

func refused(p application.PromptInfo) application.PromptInfo {
	p.Error = "not a JSON number"

	return p
}
