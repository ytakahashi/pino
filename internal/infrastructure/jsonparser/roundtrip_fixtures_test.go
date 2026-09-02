package jsonparser

import (
	"fmt"
	"strings"

	"github.com/ytakahashi/pino/internal/domain"
)

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

func jsoncRoundTripSources() map[string]string {
	return map[string]string{
		"root comments": "// before\n{\n  \"enabled\": true\n}\n// after\n",
		"members at every gap": `{
  /* first member */
  "first" /* key */ : /* value */ 1 /* before comma */, /* after comma */
  // second member
  "second": 2 // trailing
  // inside
}`,
		"nested arrays and trailing commas": `[
  /* first */ {"a": 1,},
  // second
  [2, /* after */],
  // inside
]`,
		"empty containers": `{
  "object": { /* no members */ },
  "array": [
    // no elements
  ],
}`,
		"comment before an object comma": `{
  "a": 1
  // dangling
  ,
}`,
		"comment before an array comma": `[
  1
  // dangling
  ,
  2
]`,
		"block comment before an array comma": `[
  1
  /* dangling */ , 2
]`,
		"block comment after a comma on its line": `[
  1
  , /* next */ 2
]`,
		"line comments around a comma on its line": `[
  1 // first
  // second
  , // third
  // fourth
  2
]`,
		"comments across a colon preceded by a newline": `{
  "a" // key
  : /* value */ 1
}`,
	}
}

func jsoncGapSweepSources() map[string]string {
	templates := map[string]string{
		"object":                 `{@"a"@:@1@,@"b"@:@2@}`,
		"array":                  `[@1@,@2@]`,
		"nested array in object": `{@"a"@:@[@1@,@2@]@}`,
		"nested object in array": `[@{@"a"@:@1@}@]`,
		"empty object":           `{@}`,
		"empty array":            `[@]`,
		"array trailing comma":   `[@1@,@]`,
		"object trailing comma":  `{@"a"@:@1@,@}`,
	}
	comments := map[string]string{
		"empty":                   "",
		"space":                   " ",
		"newline":                 "\n",
		"inline block":            " /* block */ ",
		"own-line block":          "\n/* block */\n",
		"block then newline":      " /* block */\n",
		"newline then block":      "\n/* block */ ",
		"inline line":             " // line\n",
		"own-line line":           "\n// line\n",
		"two line comments":       " // first\n // second\n",
		"block then line comment": "\n/* block */ // line\n",
	}

	sources := make(map[string]string)
	for templateName, template := range templates {
		parts := strings.Split(template, "@")
		for gap := range len(parts) - 1 {
			for commentName, comment := range comments {
				var src strings.Builder
				src.WriteString(parts[0])
				for i := range len(parts) - 1 {
					if i == gap {
						src.WriteString(comment)
					} else {
						src.WriteByte(' ')
					}
					src.WriteString(parts[i+1])
				}

				name := fmt.Sprintf("%s/gap %d/%s", templateName, gap, commentName)
				sources[name] = src.String()
			}
		}
	}

	return sources
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
