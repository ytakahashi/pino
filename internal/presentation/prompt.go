package presentation

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
)

// The band pino asks its questions in, drawn between the document and the
// status bar. What it asks comes from the session; how many rows that takes,
// and where on them each piece goes, is decided here.

const (
	// promptPad is the column of space kept at each end of the band, so that
	// its text does not sit against the edge of the screen.
	promptPad = " "

	// promptGap is the least space between one thing on a row and the next.
	promptGap = "  "

	// choicesPerRow is how many choices are offered before the next row. Three
	// is what fits the six types on two rows at the narrowest terminal pino
	// draws in.
	choicesPerRow = 3
)

// promptRows is how many rows the band takes, the rule above it included, and
// none at all when nothing is being asked.
//
// inputRows is how tall the box being typed into has grown, which only the
// widget holding the text can say. It is a parameter rather than something
// read here so that this stays a function of what is on the band: the layout
// is worked out from it before anything is drawn.
func promptRows(p application.PromptInfo, inputRows int) int {
	if p.Kind == application.PromptNone {
		return 0
	}

	// A notice has a fixed conclusion, acknowledgement and cause. Its detail
	// must not grow the band, or a long OS error could hide the only key that
	// dismisses it on a short terminal.
	if p.Notice != nil {
		return 3
	}

	rows := ruleRows

	switch p.Kind {
	case application.PromptText:
		rows += max(inputRows, 1)

	case application.PromptChoice:
		// The title takes a row of its own, so how tall the band is does not
		// depend on how long the question happens to be or on how wide the
		// terminal is. A band whose height moved with the width would move the
		// bottom of the document about as a window was resized.
		rows += 1 + choiceRowCount(len(p.Choices))

	case application.PromptNone:
		// Answered above. Listed so that a kind added later is reported here.
	}

	if p.Error != "" {
		rows++
	}

	return rows
}

// choiceRowCount is how many rows n choices are laid out on.
func choiceRowCount(n int) int {
	return (n + choicesPerRow - 1) / choicesPerRow
}

// inputWidth is how many columns the box has, once the title beside it has
// taken its own.
//
// The box is told this rather than being left to fill the row, so that the
// widget scrolls its own text sideways: that is what keeps the caret on screen
// without the band having to cut anything.
//
// The hint takes nothing, being on the rule above. At the narrowest terminal
// pino draws in, the keys a string prompt offers are thirty-five columns of
// the sixty: a box left with what remained would show a word at a time.
func inputWidth(p application.PromptInfo, width int) int {
	used := len(promptPad) + ansi.StringWidth(printable(p.Title)) + len(promptGap) + len(promptPad)

	return max(width-used, 1)
}

// hintFor is the keys the prompt takes, as they are written on it.
//
// They are on the band rather than in the status bar because they are the keys
// this prompt accepts: what is on screen is what the prompt is offering, and
// the bar is already saying six other things that change with the view.
func hintFor(p application.PromptInfo) string {
	switch p.Kind {
	case application.PromptText:
		if p.Multiline {
			return "Enter ok" + promptGap + "Ctrl+j newline" + promptGap + "Esc cancel"
		}

		return "Enter ok" + promptGap + "Esc cancel"

	case application.PromptChoice:
		return "Esc cancel"

	case application.PromptNone:
	}

	return ""
}

// RenderPrompt draws the band: the rule above it, what is being asked, and why
// the last answer was refused.
//
// input is the box as the widget drew it, a row at a time. It arrives already
// styled and is placed rather than rendered here, which is what keeps the
// choice of widget in one file.
func (t Theme) RenderPrompt(p application.PromptInfo, input []string, width int) []string {
	if p.Kind == application.PromptNone {
		return nil
	}

	if p.Notice != nil {
		return t.renderNotice(*p.Notice, width)
	}

	body := t.promptBody(p, input)

	rows := make([]string, 0, len(body)+2)
	rows = append(rows, t.ruleWithHint(hintFor(p), width))
	rows = append(rows, body...)

	if p.Error != "" {
		rows = append(rows, promptPad+t.PromptError.Render(printable(p.Error)))
	}

	return rows
}

// renderNotice keeps the outcome and the acknowledgement visible before the
// underlying cause. Runtime results are deliberately not normal prompts: an
// error detail has no reason to take rows away from the next action.
func (t Theme) renderNotice(n application.NoticeInfo, width int) []string {
	detailStyle := t.PromptError
	if n.Severity == application.NoticeWarning {
		detailStyle = t.PromptWarning
	}

	return []string{
		noticeRow(t.Prompt, n.Summary, width),
		noticeRow(t.Prompt, "[o] OK", width),
		noticeRow(detailStyle, n.Detail, width),
	}
}

// noticeRow makes every runtime notice row a single printable terminal row.
func noticeRow(style lipgloss.Style, text string, width int) string {
	if width <= 0 {
		return ""
	}

	return promptPad + style.Render(ansi.Truncate(printable(text), max(width-len(promptPad), 0), "…"))
}

// promptBody is the rows the question itself takes.
func (t Theme) promptBody(p application.PromptInfo, input []string) []string {
	title := t.Prompt.Render(printable(p.Title))

	switch p.Kind {
	case application.PromptText:
		return textRows(title, input)

	case application.PromptChoice:
		return t.choiceRows(title, p.Choices)

	case application.PromptNone:
	}

	return nil
}

// textRows puts the title beside the first row of the box and indents the rest
// under it, so that an answer running to several rows reads as one answer.
func textRows(title string, input []string) []string {
	lead := promptPad + title + promptGap
	under := strings.Repeat(" ", ansi.StringWidth(lead))

	if len(input) == 0 {
		return []string{lead}
	}

	rows := make([]string, 0, len(input))

	for i, line := range input {
		if i == 0 {
			rows = append(rows, lead+line)

			continue
		}

		rows = append(rows, under+line)
	}

	return rows
}

// choiceRows puts the title on a row of its own and the choices on the rows
// below it, in columns of a fixed width so that they line up under one
// another.
func (t Theme) choiceRows(title string, choices []application.Choice) []string {
	rows := make([]string, 0, 1+choiceRowCount(len(choices)))
	rows = append(rows, promptPad+title)

	cell := choiceCellWidth(choices)

	for i := 0; i < len(choices); i += choicesPerRow {
		var b strings.Builder

		b.WriteString(promptPad)

		for _, c := range choices[i:min(i+choicesPerRow, len(choices))] {
			b.WriteString(pad(t.renderChoice(c), cell))
		}

		// The padding of the last column would otherwise decide where the hint
		// on this row begins.
		rows = append(rows, strings.TrimRight(b.String(), " "))
	}

	return rows
}

// renderChoice is one answer and the key that gives it, as "[s] string".
//
// The key is drawn as brightly as the label because it is the half that has to
// be pressed; the brackets are dimmer, being punctuation around it.
func (t Theme) renderChoice(c application.Choice) string {
	return t.PromptHint.Render("[") +
		t.Prompt.Render(string(c.Key)) +
		t.PromptHint.Render("]") +
		" " + t.Prompt.Render(printable(c.Label))
}

// choiceCellWidth is the column every choice is given, which is the widest of
// them and a gap.
func choiceCellWidth(choices []application.Choice) int {
	widest := 0
	for _, c := range choices {
		widest = max(widest, ansi.StringWidth(choiceText(c)))
	}

	return widest + len(promptGap)
}

// choiceText is a choice without its styling, which is what its width is
// measured from.
func choiceText(c application.Choice) string {
	return "[" + string(c.Key) + "] " + printable(c.Label)
}

// ruleWithHint is the rule dividing the band from the document, with the keys
// the prompt takes written at the end of it.
//
// They go on the rule rather than beside what is being typed for two reasons.
// The rule is otherwise empty, while the row below it is where the answer
// goes: at sixty columns the keys a string prompt offers are thirty-five of
// them, and a box left with what remained would show a word at a time. And a
// hint that moved aside as an answer grew, or came and went as it was typed,
// would be the most restless thing on the screen.
//
// A rule too short to hold them is drawn plain. The keys go on working whether
// or not they are written down, which is what makes them the part to give up.
func (t Theme) ruleWithHint(hint string, width int) string {
	if width <= 0 {
		return ""
	}

	// A gap either side, so the words are not run into by the rule.
	room := width - ansi.StringWidth(hint) - 2*len(promptPad)
	if hint == "" || room < 1 {
		return t.RenderHorizontalRule(width)
	}

	return t.Rule.Render(strings.Repeat("─", room)) +
		promptPad + t.PromptHint.Render(hint) + promptPad
}
