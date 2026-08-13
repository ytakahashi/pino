package domain

// escapeCases returns the inputs worth escaping, kept apart so that the two
// functions are checked against the same set without sharing mutable state.
func escapeCases() map[string]struct {
	in   string
	want string // escaped, without the quotes
} {
	return map[string]struct {
		in   string
		want string
	}{
		"empty":                {in: "", want: ``},
		"plain text":           {in: "localhost", want: `localhost`},
		"a quote":              {in: `say "hi"`, want: `say \"hi\"`},
		"a backslash":          {in: `C:\tmp`, want: `C:\\tmp`},
		"the short escapes":    {in: "\b\f\n\r\t", want: `\b\f\n\r\t`},
		"a control character":  {in: "a\x00b", want: `a\u0000b`},
		"the last control one": {in: "\x1f", want: `\u001f`},
		"delete":               {in: "\x7f", want: "\x7f"},
		"kanji":                {in: "設定", want: "設定"},
		"an emoji":             {in: "🌲", want: "🌲"},
		"a solidus":            {in: "a/b", want: "a/b"},
		"only a quote":         {in: `"`, want: `\"`},
	}
}
