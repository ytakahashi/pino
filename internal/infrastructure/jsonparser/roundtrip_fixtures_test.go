package jsonparser

import "github.com/ytakahashi/pino/internal/domain"

// The fixtures here are documents to be read, written and read again. They
// are sources rather than trees: what a round trip has to survive is a file,
// and a tree built here would agree with whatever the parser happens to do
// with one.

// roundTripSources are documents worth writing back out, named by what makes
// each of them awkward.
func roundTripSources() map[string]string {
	return map[string]string{
		"an empty object":        `{}`,
		"an empty array":         `[]`,
		"a root string":          `"x"`,
		"a root number":          `42`,
		"a root null":            `null`,
		"one member":             `{"host":"localhost"}`,
		"every kind":             `{"s":"x","n":1,"t":true,"f":false,"z":null,"a":[],"o":{}}`,
		"nested containers":      `{"server":{"tls":{"port":8443}},"tags":[["a"],[]]}`,
		"an array of objects":    `[{"name":"first"},{"name":"second"}]`,
		"members in no order":    `{"b":2,"a":1,"c":3}`,
		"numbers of every kind":  `[0,-0,1.50,1e10,1E+10,1.0e-7,12345678901234567890]`,
		"keys needing escapes":   `{"a\"b":1,"c\\d":2,"e\nf":3}`,
		"strings needing quotes": `["say \"hi\"","C:\\tmp","a\tb"]`,
		"text outside ASCII":     `{"鍵":"値","emoji":"🌲"}`,
		"a deep document":        `{"a":{"b":{"c":{"d":{"e":[1,{"f":null}]}}}}}`,
	}
}

// roundTripFormats are the layouts a document is written back in. A round
// trip has to hold whichever one the file it came from used.
func roundTripFormats() map[string]domain.Format {
	return map[string]domain.Format{
		"two spaces":          {Indent: "  ", Newline: "\n", TrailingNL: true},
		"four spaces":         {Indent: "    ", Newline: "\n", TrailingNL: true},
		"tabs":                {Indent: "\t", Newline: "\n", TrailingNL: true},
		"no indent":           {Indent: "", Newline: "\n", TrailingNL: true},
		"CRLF":                {Indent: "  ", Newline: "\r\n", TrailingNL: true},
		"no trailing newline": {Indent: "  ", Newline: "\n", TrailingNL: false},
	}
}

// canonicalSources are already laid out the way pino writes: reading one and
// saving it must produce the very bytes it came from.
func canonicalSources() map[string]string {
	return map[string]string{
		"two spaces and LF":     "{\n  \"host\": \"localhost\",\n  \"port\": 8080\n}\n",
		"four spaces":           "{\n    \"server\": {\n        \"port\": 8443\n    }\n}\n",
		"tabs":                  "{\n\t\"server\": {\n\t\t\"port\": 8443\n\t}\n}\n",
		"CRLF":                  "{\r\n  \"host\": \"localhost\"\r\n}\r\n",
		"no trailing newline":   "{\n  \"host\": \"localhost\"\n}",
		"an array of objects":   "[\n  {\n    \"name\": \"first\"\n  },\n  {\n    \"name\": \"second\"\n  }\n]\n",
		"empty containers":      "{\n  \"empty\": {},\n  \"none\": []\n}\n",
		"a root number":         "42\n",
		"a document of nothing": "{}\n",
	}
}
