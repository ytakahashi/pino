package presentation

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
)

// fieldNameWidth is the column the values of the stacked pane begin in.
//
// It is fixed at the widest name any field can have rather than at the widest
// among the fields on screen, so that the values stay put as the selection
// moves. A column that shifted whenever the cursor passed from a value to a
// container would be the same restlessness the pane avoids by naming one field
// Value or Children instead of showing both.
const fieldNameWidth = len("Children") + 2

// inspectorPad is the column of space kept at the left of a pane, so that its
// text does not sit against the rule beside it.
const inspectorPad = " "

// inspectorField is one thing the pane says about the selected node.
type inspectorField struct {
	// Name is what pino calls it. Which name a field gets is sometimes the
	// answer itself: Key or Index says what the document turned out to be.
	Name string

	// Value is the text, and the Role to colour it by when the text is the
	// document's own.
	Value application.Span

	// Styled says whether Value carries a Role, since the zero Role is a real
	// one rather than an absence.
	//
	// One field is the document speaking and the rest are pino speaking about
	// it, and the pane draws that difference: only what the document itself
	// holds gets the colours the document is drawn in.
	Styled bool
}

// inspectorFields is what the pane says about a node, in the order it says it.
//
// The two panes are built from the same list. What differs between them is how
// a field is laid out, not which fields there are, so a pane cannot come to
// describe a node differently from the other.
//
// An empty list is the answer when nothing is selected. The type is what says
// so: the pointer of the root is empty, and so is the one reported with no
// document open.
func inspectorFields(info application.InspectorInfo) []inspectorField {
	if info.Type == "" {
		return nil
	}

	fields := []inspectorField{
		{Name: "Path", Value: plainSpan(printable(pointerLabel(info.Pointer)))},
		{Name: "Type", Value: plainSpan(info.Type)},
	}

	// Children and Value share a place because one of them is always empty: a
	// container has no value that fits on a line, and a value has no children.
	// Two fields would leave one of them permanently blank and the other
	// moving up and down as the selection changed.
	if info.Container {
		fields = append(fields, inspectorField{
			Name:  "Children",
			Value: plainSpan(strconv.Itoa(info.Children)),
		})
	} else {
		fields = append(fields, inspectorField{Name: "Value", Value: info.Value, Styled: true})
	}

	// The name the node has within its parent, and what kind of name that is.
	// A pointer alone cannot tell the first element of an array from the
	// member "0" of an object; this is the one place on screen that says.
	switch info.Naming {
	case application.NamedKey:
		fields = append(fields, inspectorField{Name: "Key", Value: plainSpan(printable(info.Label))})

	case application.NamedIndex:
		fields = append(fields, inspectorField{Name: "Index", Value: plainSpan(printable(info.Label))})

	case application.NamedNone:
		// The root is a member of nothing, so it has no name to give.
	}

	return fields
}

func plainSpan(text string) application.Span { return application.Span{Text: text} }

// RenderInspectorPane draws the pane that stands beside the tree: exactly
// height rows of exactly width columns.
//
// A field takes two rows, its name above its value, because the pane is narrow
// and the two side by side would leave a pointer nowhere to go. A blank row
// between one field and the next is what keeps a wrapped value from reading as
// the beginning of the next field.
//
// A value too long for the width wraps, and the pane is cut at its height. The
// order of the fields and the space between them therefore do not change with
// the size of the terminal, so the eye can go on finding a field where it was;
// what a short terminal costs is the last field rather than the shape of all
// of them. Only a terminal below fourteen rows cuts anything.
func (t Theme) RenderInspectorPane(info application.InspectorInfo, width, height int) []string {
	text := max(width-len(inspectorPad), 0)

	var rows []string

	for i, f := range inspectorFields(info) {
		if i > 0 {
			rows = append(rows, "")
		}

		rows = append(rows, inspectorPad+t.FieldName.Render(ansi.Truncate(f.Name, text, "")))

		// Spaces are kept where a line begins with one. Dropping them is what
		// wrapping prose wants, and the opposite of what this pane is for: a
		// value read back here has to be the value, and a key that holds two
		// spaces is a different key from one that holds one. The width of a
		// line still counts them, so a kept space costs a column as any other
		// character does.
		for _, line := range strings.Split(ansi.Hardwrap(f.Value.Text, max(text, 1), true), "\n") {
			rows = append(rows, inspectorPad+t.fieldValue(f, line))
		}
	}

	return fitRows(rows, width, height)
}

// RenderInspectorStrip draws the pane that goes under the tree on a terminal
// too narrow to put one beside it: exactly height rows of exactly width
// columns, one field to a row.
//
// A row apiece rather than two, because rows are what a short screen is short
// of while a wide one has columns to spare. The names are held to one column
// so that the values line up under one another.
func (t Theme) RenderInspectorStrip(info application.InspectorInfo, width, height int) []string {
	text := max(width-len(inspectorPad), 0)

	var rows []string

	for _, f := range inspectorFields(info) {
		name := t.FieldName.Render(pad(ansi.Truncate(f.Name, text, ""), fieldNameWidth))
		row := name + t.fieldValue(f, ansi.Truncate(f.Value.Text, max(text-fieldNameWidth, 0), ""))

		rows = append(rows, inspectorPad+row)
	}

	return fitRows(rows, width, height)
}

// fieldValue draws text as the field would have it: in the document's own
// colours where the field holds the document's own text, and plainly where it
// holds pino's.
func (t Theme) fieldValue(f inspectorField, text string) string {
	if !f.Styled {
		return text
	}

	return t.style(f.Value.Role).Render(text)
}

// RenderVerticalRule is the column between the document and the pane beside it.
func (t Theme) RenderVerticalRule() string { return t.Rule.Render("│") }

// RenderHorizontalRule is the row between the document and the pane below it.
func (t Theme) RenderHorizontalRule(width int) string {
	if width <= 0 {
		return ""
	}

	return t.Rule.Render(strings.Repeat("─", width))
}

// fitRows brings rows to exactly height rows of exactly width columns.
//
// A pane is a block rather than a list of whatever it happened to have to say,
// because it is laid beside the document a row at a time: a row that came up
// short would let the document's own row show through beside it, and one row
// too many would push the status bar off the screen.
func fitRows(rows []string, width, height int) []string {
	fitted := make([]string, 0, max(height, 0))

	for i := range max(height, 0) {
		row := ""
		if i < len(rows) {
			row = rows[i]
		}

		fitted = append(fitted, pad(ansi.Truncate(row, max(width, 0), ""), max(width, 0)))
	}

	return fitted
}

// pad fills a row out to width columns. A row already that wide is left alone.
func pad(row string, width int) string {
	if gap := width - ansi.StringWidth(row); gap > 0 {
		return row + strings.Repeat(" ", gap)
	}

	return row
}
