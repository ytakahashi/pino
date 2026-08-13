package domain

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
