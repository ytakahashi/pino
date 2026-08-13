package domain

import (
	"errors"
	"strings"
	"testing"
)

// The spelling a value is typed in, and reading back what was typed.
//
// What matters most here is the round trip: a value opened for editing and
// committed without a keystroke has to come back exactly as it went, or a
// document is changed by being looked at.

// awkwardStrings are the values that go wrong when a spelling is not exact.
// Every one of them holds something that cannot be typed, cannot be seen, or
// means something else to whoever reads it back.
func awkwardStrings() map[string]string {
	return map[string]string{
		"nothing at all":         "",
		"plain text":             "localhost",
		"a tab":                  "a\tb",
		"a carriage return":      "a\rb",
		"a line break":           "a\nb",
		"a form feed":            "a\fb",
		"a backspace":            "a\bb",
		"a null":                 "a\x00b",
		"a bell":                 "a\x07b",
		"the last C0":            "a\x1fb",
		"delete":                 "a\x7fb",
		"a C1 control":           "a\u0085b",
		"the last C1":            "a\u009fb",
		"a replacement char":     "a\ufffdb",
		"a quotation mark":       `he said "hi"`,
		"a backslash":            `C:\Users\pino`,
		"something like escapes": `\t \u0041 \\`,
		"text in another script": "こんにちは",
		"an emoji":               "🎉",
		"several at once":        "a\t\"b\"\\c\nd\x00e🎉",
		"only breaks":            "\n\n\n",
	}
}

func TestTypingAValueAndCommittingItLeavesItAlone(t *testing.T) {
	t.Parallel()

	for name, value := range awkwardStrings() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for spelling, typed := range map[string]string{
				"as several rows": EditableText(value),
				"as one row":      EditableLine(value),
			} {
				got, err := ParseEditableText(typed)
				if err != nil {
					t.Fatalf("%s: ParseEditableText(%q): %v", spelling, typed, err)
				}

				if got != value {
					t.Errorf("%s: %q came back as %q", spelling, value, got)
				}
			}
		})
	}
}

func TestTheSpellingHidesWhatCannotBeTyped(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value string
		text  string
		line  string
	}{
		// A break is drawn as a break in a box with rows to draw it in, and as
		// an escape where the whole value has to be one row.
		"a line break": {value: "a\nb", text: "a\nb", line: `a\nb`},

		// Everything else is spelled the way the document spells it.
		"a tab":              {value: "a\tb", text: `a\tb`, line: `a\tb`},
		"a carriage return":  {value: "a\rb", text: `a\rb`, line: `a\rb`},
		"a quotation mark":   {value: `"`, text: `\"`, line: `\"`},
		"a backslash":        {value: `\`, text: `\\`, line: `\\`},
		"a bell":             {value: "\x07", text: `\u0007`, line: `\u0007`},
		"text in any script": {value: "日本語 🎉", text: "日本語 🎉", line: "日本語 🎉"},

		// Two the encoder leaves alone, because JSON does not require them to
		// be escaped. Neither can be typed or told apart on a screen.
		"delete":              {value: "\x7f", text: `\u007f`, line: `\u007f`},
		"a C1 control":        {value: "\u0085", text: `\u0085`, line: `\u0085`},
		"a replacement char":  {value: "\ufffd", text: `\ufffd`, line: `\ufffd`},
		"nothing to hide":     {value: "localhost", text: "localhost", line: "localhost"},
		"an empty value":      {value: "", text: "", line: ""},
		"a break and a break": {value: "\n\n", text: "\n\n", line: `\n\n`},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := EditableText(tt.value); got != tt.text {
				t.Errorf("EditableText(%q) = %q, want %q", tt.value, got, tt.text)
			}

			if got := EditableLine(tt.value); got != tt.line {
				t.Errorf("EditableLine(%q) = %q, want %q", tt.value, got, tt.line)
			}
		})
	}
}

// A line is one row however many breaks the value holds, which is what makes
// it the spelling to fall back on when a box cannot hold the rows.
func TestALineIsOneRow(t *testing.T) {
	t.Parallel()

	value := strings.Repeat("row\n", 1000)

	if got := strings.Count(EditableLine(value), "\n"); got != 0 {
		t.Errorf("EditableLine() left %d breaks, want none", got)
	}

	if got := strings.Count(EditableText(value), "\n"); got != 1000 {
		t.Errorf("EditableText() left %d breaks, want 1000", got)
	}
}

func TestReadingBackWhatWasTyped(t *testing.T) {
	t.Parallel()

	// Spellings a person may type that the two above never produce.
	tests := map[string]struct {
		typed string
		want  string
	}{
		"an escaped solidus":         {`a\/b`, "a/b"},
		"a character with no need":   {`\u0041`, "A"},
		"a pair above the BMP":       {`\ud83c\udf89`, "🎉"},
		"a break where \\n would do": {"a\nb", "a\nb"},
		"escapes in a row":           {`\t\r\n`, "\t\r\n"},
		"a backslash before text":    {`\\t`, `\t`},
		"nothing":                    {"", ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseEditableText(tt.typed)
			if err != nil {
				t.Fatalf("ParseEditableText(%q): %v", tt.typed, err)
			}

			if got != tt.want {
				t.Errorf("ParseEditableText(%q) = %q, want %q", tt.typed, got, tt.want)
			}
		})
	}
}

func TestTextThatCannotBeReadBack(t *testing.T) {
	t.Parallel()

	// Every one of these is something a person can type, so each has to be
	// refused with a reason rather than guessed at.
	tests := map[string]string{
		"a backslash at the end":      `abc\`,
		"an escape nothing means":     `a\qb`,
		"an escaped tab character":    "a\\\tb",
		"a short \\u":                 `a\u12b`,
		"a \\u of no digits":          `a\uzzzz`,
		"a \\u at the end":            `a\u`,
		"half a character":            `a\ud83cb`,
		"half a character at the end": `a\ud83c`,
		"two first halves":            `\ud83c\ud83c`,
		"a second half alone":         `\udf89`,
	}

	for name, typed := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseEditableText(typed)
			if err == nil {
				t.Fatalf("ParseEditableText(%q) = %q, want a refusal", typed, got)
			}

			var invalid *InvalidEscapeError
			if !errors.As(err, &invalid) {
				t.Fatalf("ParseEditableText(%q) = %T, want an *InvalidEscapeError", typed, err)
			}

			if invalid.Reason == "" {
				t.Error("the refusal does not say why")
			}

			if invalid.Index < 0 || invalid.Index >= len(typed) {
				t.Errorf("the refusal points at %d, which is outside %q", invalid.Index, typed)
			}
		})
	}
}

// What is read back is what a document may hold: the constructor checks the
// same thing, and the two must not disagree about a value that came from here.
func TestWhatIsReadBackCanEnterADocument(t *testing.T) {
	t.Parallel()

	for name, value := range awkwardStrings() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			read, err := ParseEditableText(EditableText(value))
			if err != nil {
				t.Fatalf("ParseEditableText: %v", err)
			}

			if _, err := NewString(read); err != nil {
				t.Errorf("NewString(%q): %v", read, err)
			}
		})
	}
}
