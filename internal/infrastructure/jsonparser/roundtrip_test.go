package jsonparser

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// A round trip is what pino checks before it overwrites a file: the document
// is encoded, the bytes are parsed again, and the two trees are compared. It
// is checked here rather than in domain because the parser is an adapter, and
// domain may not reach one — the encoder can only be shown to agree with a
// parser where both are in scope.

func TestEncodingADocumentAndReadingItBackYieldsTheSameDocument(t *testing.T) {
	t.Parallel()

	for name, src := range roundTripSources() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			original := parseStrict(t, src)

			for formatName, format := range roundTripFormats() {
				t.Run(formatName, func(t *testing.T) {
					t.Parallel()

					written := domain.Encode(original, format)
					reparsed := parseStrict(t, string(written))

					if !domain.Equal(original, reparsed) {
						t.Errorf("reading back\n%s\ngave %s, want %s",
							written, dump(reparsed), dump(original))
					}
				})
			}
		})
	}
}

func TestEncodingAJSONCDocumentConvergesWithoutLosingComments(t *testing.T) {
	t.Parallel()

	for name, src := range jsoncRoundTripSources() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			original, err := New().Parse([]byte(src), domain.JSONC)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			for formatName, format := range roundTripFormats() {
				t.Run(formatName, func(t *testing.T) {
					t.Parallel()

					first := domain.Encode(original, format)
					reparsed, err := New().Parse(first, domain.JSONC)
					if err != nil {
						t.Fatalf("Parse encoded document: %v", err)
					}
					if !domain.Equal(original, reparsed) {
						t.Errorf("reading back\n%s\ngave %s, want %s", first, dump(reparsed), dump(original))
					}

					if second := domain.Encode(reparsed, format); string(second) != string(first) {
						t.Errorf("second encoding wrote\n%s\nwant\n%s", second, first)
					}
				})
			}
		})
	}
}

func TestCommentsAtEveryJSONGapSurviveARoundTrip(t *testing.T) {
	t.Parallel()

	format := domain.Format{Indent: "  ", Newline: "\n", TrailingNL: true}
	for name, src := range jsoncGapSweepSources() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			original, err := New().Parse([]byte(src), domain.JSONC)
			if err != nil {
				t.Fatalf("Parse(%q): %v", src, err)
			}

			first := domain.Encode(original, format)
			reparsed, err := New().Parse(first, domain.JSONC)
			if err != nil {
				t.Fatalf("Parse encoded document: %v", err)
			}
			if !domain.Equal(original, reparsed) {
				t.Errorf("reading back\n%s\ngave %s, want %s", first, dump(reparsed), dump(original))
			}

			if second := domain.Encode(reparsed, format); string(second) != string(first) {
				t.Errorf("second encoding wrote\n%s\nwant\n%s", second, first)
			}
		})
	}
}

// The point of following the source's layout is that saving a document
// nobody reformatted leaves the file alone. Nothing but the bytes can say
// that: two trees compare equal however the whitespace between them moved.
func TestACanonicallyLaidOutSourceIsWrittenBackByteForByte(t *testing.T) {
	t.Parallel()

	for name, src := range canonicalSources() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			node := parseStrict(t, src)

			if got := string(domain.Encode(node, domain.DetectFormat([]byte(src)))); got != src {
				t.Errorf("saving an untouched document wrote %q, wanted the source %q", got, src)
			}
		})
	}
}

// Escapes are undone on the way in and reapplied by the rules of RFC 8259, so
// a source spelling a character some other admissible way comes back spelled
// pino's way. The bytes differ; the document does not, which is what the
// check in front of a save is asking about.
func TestASourceSpelledAnotherWayIsTheSameDocument(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"a unicode escape":       `["` + esc(0x00E9) + `"]`,
		"uppercase hexadecimal":  `["` + escUpper(0x00E9) + `"]`,
		"an escaped solidus":     `["a\/b"]`,
		"an escaped surrogate":   `["` + esc(0xD83D) + esc(0xDE00) + `"]`,
		"whitespace between all": "{\n\n  \"a\"  :\t1\n\n}",
	}

	format := domain.Format{Indent: "  ", Newline: "\n", TrailingNL: true}

	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			original := parseStrict(t, src)
			written := domain.Encode(original, format)

			if string(written) == src {
				t.Fatalf("the source was written back unchanged, so nothing is being respelled: %q", src)
			}

			if !domain.Equal(original, parseStrict(t, string(written))) {
				t.Errorf("respelling %q as %q changed the document", src, written)
			}
		})
	}
}
