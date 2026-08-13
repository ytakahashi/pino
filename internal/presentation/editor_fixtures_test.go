package presentation

import (
	"strconv"
	"strings"

	"github.com/ytakahashi/pino/internal/application"
)

// box is an editor seeded the way an effect from the session would seed it.
//
// A value with a break in it is offered on one line as well, which is what the
// session does: the box falls back to it when it cannot hold the rows.
func box(text string, multiline bool) editor {
	e, ok := newEditor(
		DefaultTheme(),
		application.EffectBeginInput{
			Text:      text,
			OneLine:   strings.ReplaceAll(text, "\n", `\n`),
			Multiline: multiline,
		},
		40,
	)
	if !ok {
		// Every value in these tests is one a box can hold; a box that did not
		// hold one is the test's own mistake and not something to go on with.
		panic("the box did not take " + strconv.Quote(text))
	}

	return e
}

// awkwardSpellings are values as the session spells them for editing, along
// with what a document holds when one is committed unchanged.
//
// They are written out rather than produced by calling the domain, so that
// this says what the box has to hold rather than agreeing with whatever the
// spelling happens to be.
func awkwardSpellings() map[string]string {
	return map[string]string{
		"plain text":         "localhost",
		"a tab":              `a\tb`,
		"a carriage return":  `a\rb`,
		"a null":             `a\u0000b`,
		"a bell":             `a\u0007b`,
		"delete":             `a\u007fb`,
		"a C1 control":       `a\u0085b`,
		"a replacement char": `a\ufffdb`,
		"a quotation mark":   `he said \"hi\"`,
		"a backslash":        `C:\\Users\\pino`,
		"an emoji":           "🎉",
		"a line break":       "a\nb",
		"nothing at all":     "",
	}
}
