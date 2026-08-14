package presentation

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
)

func TestInspectorFieldsDescribeEveryValue(t *testing.T) {
	tests := map[string]struct {
		info application.InspectorInfo
		want []string // "Name=Value"
	}{
		"a value": {
			info: scalarInfo(),
			want: []string{
				"Path=/server/port", "Type=number", "Value=8080", "Key=port",
				"Keys=Enter t A d r",
			},
		},

		// A container says how many children it holds where a value says what
		// it is: one of the two is always empty, so they share a place.
		"a container": {
			info: containerInfo(),
			want: []string{
				"Path=/server", "Type=object", "Children=3", "Key=server",
				"Keys=Enter t a A d r",
			},
		},

		"an empty container": {
			info: application.InspectorInfo{
				Pointer: "/opts", Type: "object", Container: true, Children: 0,
				Label: "opts", Naming: application.NamedKey,
			},
			want: []string{
				"Path=/opts", "Type=object", "Children=0", "Key=opts",
				"Keys=Enter t a A d r",
			},
		},

		// The name of the last field is the answer to a question a pointer
		// cannot settle on its own.
		"an element of an array": {
			info: application.InspectorInfo{
				Pointer: "/features/0", Type: "string",
				Value: application.Span{Text: `"search"`, Role: application.RoleStringValue},
				Label: "0", Naming: application.NamedIndex,
			},
			want: []string{
				"Path=/features/0", "Type=string", `Value="search"`, "Index=0",
				"Keys=Enter t A d",
			},
		},

		// The root is a member of nothing, so it is not named at all.
		"the root": {
			info: application.InspectorInfo{
				Pointer: "", Type: "object", Container: true, Children: 2,
				Naming: application.NamedNone,
			},
			want: []string{"Path=/", "Type=object", "Children=2", "Keys=Enter t a"},
		},

		// A member whose key is empty is still named, unlike the root.
		"a member with an empty key": {
			info: application.InspectorInfo{
				Pointer: "/", Type: "null",
				Value:  application.Span{Text: "null", Role: application.RoleNullValue},
				Naming: application.NamedKey,
			},
			want: []string{"Path=/", "Type=null", "Value=null", "Key=", "Keys=Enter t A d r"},
		},

		// Nothing selected is nothing to say. The type is what reports that,
		// since the root's pointer is empty too.
		"nothing selected": {info: application.InspectorInfo{}, want: nil},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fields := inspectorFields(tc.info)

			got := make([]string, 0, len(fields))
			for _, f := range fields {
				got = append(got, f.Name+"="+f.Value.Text)
			}

			if strings.Join(got, " | ") != strings.Join(tc.want, " | ") {
				t.Errorf("inspectorFields() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A key holding a control character would break a row in two or be obeyed by
// the terminal. The pointer carries the same key and is guarded the same way.
func TestInspectorFieldsMakeOutsideTextPrintable(t *testing.T) {
	info := application.InspectorInfo{
		Pointer: "/nl\nhere",
		Type:    "number",
		Value:   application.Span{Text: "1", Role: application.RoleNumberValue},
		Label:   "nl\nhere",
		Naming:  application.NamedKey,
	}

	for _, f := range inspectorFields(info) {
		if strings.ContainsAny(f.Value.Text, "\n\r\x1b") {
			t.Errorf("field %s holds %q, want it made printable", f.Name, f.Value.Text)
		}
	}
}

// Only the value is the document speaking. Everything else is pino describing
// the document, and the difference is drawn in colour.
func TestInspectorFieldsStyleOnlyTheValue(t *testing.T) {
	for _, f := range inspectorFields(scalarInfo()) {
		if want := f.Name == "Value"; f.Styled != want {
			t.Errorf("field %s has Styled = %t, want %t", f.Name, f.Styled, want)
		}
	}

	// A container has no value, so nothing in its pane is the document's own.
	for _, f := range inspectorFields(containerInfo()) {
		if f.Styled {
			t.Errorf("field %s of a container is drawn in the document's colours", f.Name)
		}
	}
}

// The pane beside the tree puts a field's name above its value, with a blank
// row between one field and the next.
func TestRenderInspectorPaneDrawsLabelledRows(t *testing.T) {
	got := plain(DefaultTheme().RenderInspectorPane(scalarInfo(), 32, 17))

	want := []string{
		" Path",
		" /server/port",
		"",
		" Type",
		" number",
		"",
		" Value",
		" 8080",
		"",
		" Key",
		" port",
		"",
		" Keys",
		" Enter t A d r",
		"", "", "",
	}

	for i, w := range want {
		if got[i] != w {
			t.Errorf("row %d is %q, want %q", i, got[i], w)
		}
	}
}

// A value too long for the width is wrapped rather than cut: the pane is where
// a value shortened on a row is read back in full.
func TestRenderInspectorPaneWrapsLongValues(t *testing.T) {
	const width = 20

	value := strings.Repeat("x", 40)

	info := scalarInfo()
	info.Value = application.Span{Text: `"` + value + `"`, Role: application.RoleStringValue}

	got := plain(DefaultTheme().RenderInspectorPane(info, width, 20))

	joined := strings.Join(got, "")
	if !strings.Contains(strings.ReplaceAll(joined, " ", ""), value) {
		t.Errorf("the value was not drawn whole: %v", got)
	}
}

// What does not fit in the height is left off the bottom, so that the fields
// above it stay where the eye last found them.
func TestRenderInspectorPaneCutsAtTheHeight(t *testing.T) {
	got := plain(DefaultTheme().RenderInspectorPane(scalarInfo(), 32, 5))

	want := []string{" Path", " /server/port", "", " Type", " number"}

	for i, w := range want {
		if got[i] != w {
			t.Errorf("row %d is %q, want %q", i, got[i], w)
		}
	}
}

// The stacked pane puts a field on a row of its own, the values lined up in
// one column.
func TestRenderInspectorStripDrawsCompactFields(t *testing.T) {
	got := plain(DefaultTheme().RenderInspectorStrip(scalarInfo(), 60, 5))

	want := []string{
		" Path      /server/port",
		" Type      number",
		" Value     8080",
		" Key       port",
		" Keys      Enter t A d r",
	}

	for i, w := range want {
		if got[i] != w {
			t.Errorf("row %d is %q, want %q", i, got[i], w)
		}
	}
}

// The column the values begin in does not move with the name in front of them,
// so that they stay put as the selection passes from a value to a container.
func TestRenderInspectorStripLinesTheValuesUp(t *testing.T) {
	value := plain(DefaultTheme().RenderInspectorStrip(scalarInfo(), 60, 5))
	container := plain(DefaultTheme().RenderInspectorStrip(containerInfo(), 60, 5))

	at := func(row string) int { return strings.Index(row, strings.TrimSpace(row[10:])) }

	for i := range 5 {
		if got, want := at(container[i]), at(value[i]); got != want {
			t.Errorf("row %d of a container begins at column %d, want %d", i, got, want)
		}
	}
}

func TestRenderInspectorStripCutsAvailableKeysAtThePaneWidth(t *testing.T) {
	got := plain(DefaultTheme().RenderInspectorStrip(containerInfo(), 18, 5))

	if got, want := got[4], " Keys      Enter t"; got != want {
		t.Errorf("the Keys row is %q, want %q", got, want)
	}
}

// Whatever it is asked for, a pane is a block: the rows it is laid beside have
// to meet a row of it, and one row too many would push the status bar off the
// screen.
func TestInspectorPanesAreExactlyTheSizeAsked(t *testing.T) {
	infos := map[string]application.InspectorInfo{
		"a value":         scalarInfo(),
		"a container":     containerInfo(),
		"nothing at all":  {},
		"a long value":    longValueInfo(),
		"a control chara": {Pointer: "/a\nb", Type: "null", Label: "a\nb", Naming: application.NamedKey},
	}

	sizes := []struct{ width, height int }{
		{32, 14}, {32, 1}, {32, 0}, {1, 5}, {0, 5}, {80, 4}, {8, 20},
	}

	for name, info := range infos {
		for _, size := range sizes {
			for kind, pane := range map[string][]string{
				"beside": DefaultTheme().RenderInspectorPane(info, size.width, size.height),
				"below":  DefaultTheme().RenderInspectorStrip(info, size.width, size.height),
			} {
				if len(pane) != max(size.height, 0) {
					t.Errorf("%s, %s, %dx%d: %d rows, want %d",
						name, kind, size.width, size.height, len(pane), size.height)
				}

				for i, row := range pane {
					if got := lipgloss.Width(row); got != max(size.width, 0) {
						t.Errorf("%s, %s, %dx%d: row %d is %d wide, want %d",
							name, kind, size.width, size.height, i, got, size.width)
					}
				}
			}
		}
	}
}

func TestRenderHorizontalRuleFillsTheWidth(t *testing.T) {
	if got := ansi.Strip(DefaultTheme().RenderHorizontalRule(5)); got != "─────" {
		t.Errorf("RenderHorizontalRule(5) = %q, want five rules", got)
	}

	if got := DefaultTheme().RenderHorizontalRule(0); got != "" {
		t.Errorf("RenderHorizontalRule(0) = %q, want nothing", got)
	}

	if got := DefaultTheme().RenderHorizontalRule(-1); got != "" {
		t.Errorf("RenderHorizontalRule(-1) = %q, want nothing", got)
	}
}

func TestRenderVerticalRuleDrawsOneCell(t *testing.T) {
	if got := ansi.Strip(DefaultTheme().RenderVerticalRule()); got != "│" {
		t.Errorf("RenderVerticalRule() = %q, want one rule", got)
	}
}

// A wrapped field read back row by row is the field, exactly.
//
// Wrapping prose drops a space that lands at the start of a line, which is
// right for prose and wrong here: this pane is where a value shortened on a
// row is checked, and a key holding two spaces is a different key from one
// holding one.
func TestRenderInspectorPaneKeepsSpacesAcrossTheWrap(t *testing.T) {
	// One column of padding and seven of text, so that a field of eleven
	// characters wraps with a space on the boundary.
	const width = 8

	tests := map[string]struct {
		info  application.InspectorInfo
		field string
		want  string
	}{
		// The space is the eighth character, so it begins the second row.
		"a value": {
			info: application.InspectorInfo{
				Pointer: "/a", Type: "string",
				Value: application.Span{Text: `"abcdef gh"`, Role: application.RoleStringValue},
				Label: "a", Naming: application.NamedKey,
			},
			field: "Value", want: `"abcdef gh"`,
		},

		// Two spaces in a row, the second of which begins a line.
		"a value holding two spaces": {
			info: application.InspectorInfo{
				Pointer: "/a", Type: "string",
				Value: application.Span{Text: `"abcde  fg"`, Role: application.RoleStringValue},
				Label: "a", Naming: application.NamedKey,
			},
			field: "Value", want: `"abcde  fg"`,
		},

		// A key can hold spaces too, and is drawn unquoted, so one lost at the
		// wrap would leave the pane naming a member the document does not have.
		"a key": {
			info: application.InspectorInfo{
				Pointer: "/x", Type: "null",
				Value: application.Span{Text: "null", Role: application.RoleNullValue},
				Label: "keyabc de", Naming: application.NamedKey,
			},
			field: "Key", want: "keyabc de",
		},

		// And so can a pointer, whose tokens are the document's own keys.
		"a pointer": {
			info: application.InspectorInfo{
				Pointer: "/abcdef gh", Type: "null",
				Value: application.Span{Text: "null", Role: application.RoleNullValue},
				Label: "abcdef gh", Naming: application.NamedKey,
			},
			field: "Path", want: "/abcdef gh",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Read back with the filling still on, since a space at the end of
			// a wrapped row is part of the field rather than padding.
			got := stripped(DefaultTheme().RenderInspectorPane(tc.info, width, 30))

			at := slices.IndexFunc(got, func(row string) bool {
				return strings.TrimRight(row, " ") == inspectorPad+tc.field
			})
			if at < 0 {
				t.Fatalf("the pane holds no %s field: %v", tc.field, got)
			}

			// The rows of the field are the ones up to the blank row that
			// divides it from the next.
			var b strings.Builder

			for _, row := range got[at+1:] {
				if strings.TrimSpace(row) == "" {
					break
				}

				b.WriteString(strings.TrimPrefix(row, inspectorPad))
			}

			// Only the last row of a field is filled out, so one trim at the
			// end drops the padding without touching the field's own spaces.
			if read := strings.TrimRight(b.String(), " "); read != tc.want {
				t.Errorf("the %s field reads back as %q, want %q", tc.field, read, tc.want)
			}
		})
	}
}
