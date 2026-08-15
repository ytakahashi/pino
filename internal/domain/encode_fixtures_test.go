package domain

import "testing"

// plainFormat is two spaces and LF with nothing at the end, so that a table
// of expected documents says what the layout does and not where the file
// stops. The trailing newline has a test of its own.
func plainFormat() Format {
	return Format{Indent: "  ", Newline: "\n", TrailingNL: false}
}

// everyKind is a document holding one of each kind of value, including the
// two containers that have nothing in them.
//
//	{
//	  "host": "localhost",
//	  "port": 8080,
//	  "debug": false,
//	  "extra": null,
//	  "tags": ["a", "b"],
//	  "server": {"tls": {"port": 8443}},
//	  "empty": {},
//	  "none": []
//	}
func everyKind(t *testing.T) *Object {
	t.Helper()

	return obj(t,
		Member{Key: "host", Value: str(t, "localhost")},
		Member{Key: "port", Value: NewNumber("8080")},
		Member{Key: "debug", Value: NewBool(false)},
		Member{Key: "extra", Value: NewNull()},
		Member{Key: "tags", Value: NewArray([]Node{str(t, "a"), str(t, "b")})},
		Member{Key: "server", Value: obj(t,
			Member{Key: "tls", Value: obj(t, Member{Key: "port", Value: NewNumber("8443")})},
		)},
		Member{Key: "empty", Value: obj(t)},
		Member{Key: "none", Value: NewArray(nil)},
	)
}

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
