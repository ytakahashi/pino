package domain

import "bytes"

// Format is the layout used when writing a document back out.
//
// pino re-renders the whole document on save, so a layout that differs from
// the original turns a one-line change into a whole-file diff. Reproducing
// what the source already used keeps the diff to what the user actually
// changed.
//
// The fields are exported, unlike those of Trivia and Path: a Format holds
// only a string and a bool, so a copy shares nothing with the original and
// writing to one cannot reach a document.
//
// The zero value is not usable as-is; start from DefaultFormat or
// DetectFormat.
type Format struct {
	// Indent is one level of indentation, e.g. "  " or "\t". It must consist
	// of whitespace. Empty means no indentation.
	Indent string

	// Newline ends each line: "\n" or "\r\n".
	Newline string

	// TrailingNL reports whether the document ends with a newline.
	TrailingNL bool
}

// DefaultFormat is what pino writes when there is no source to copy from.
//
// A document created from scratch never reaches DetectFormat, because it has
// no bytes to inspect; whoever opens a path that does not exist starts from
// this instead.
func DefaultFormat() Format {
	return Format{
		Indent:     "  ",
		Newline:    "\n",
		TrailingNL: true,
	}
}

// DetectFormat works out the layout of src.
//
// Each aspect is taken from src where it shows, and from DefaultFormat where
// it does not. A source that ends without a newline is a finding rather than
// an absence, so that is carried over: adding one back on save would show up
// as a change the user did not make.
//
// Empty input is the one case with nothing to find at all, and yields
// DefaultFormat unchanged. Reading "no trailing newline" out of zero bytes
// would be an inference dressed up as a detection.
//
// The caller may still override the result: an explicit --indent wins over
// whatever the file uses.
func DetectFormat(src []byte) Format {
	if len(src) == 0 {
		return DefaultFormat()
	}

	format := DefaultFormat()
	format.Newline = detectNewline(src)
	format.TrailingNL = bytes.HasSuffix(src, []byte("\n"))

	if indent, ok := detectIndent(src); ok {
		format.Indent = indent
	}

	return format
}

// detectNewline reports the line ending in use, from the first one present.
func detectNewline(src []byte) string {
	i := bytes.IndexByte(src, '\n')
	if i < 0 {
		return DefaultFormat().Newline
	}

	if i > 0 && src[i-1] == '\r' {
		return "\r\n"
	}

	return "\n"
}

// detectIndent returns the leading whitespace of the first indented line.
//
// That line sits one level below the root in any conventionally formatted
// document, so its whitespace is one level of indentation. A document whose
// first indented line happens to be deeper yields a wider unit than the source
// really uses, which changes how the file is laid out but never its meaning.
//
// The first line is skipped: it holds the opening of the root value, and
// whitespace in front of that is not indentation.
//
// Scanning line by line is safe without tracking string boundaries, because
// JSON forbids unescaped control characters inside a string. Every line break
// in a valid document is therefore structural, never part of a value.
func detectIndent(src []byte) (string, bool) {
	i := bytes.IndexByte(src, '\n')
	if i < 0 {
		return "", false
	}

	for rest := src[i+1:]; len(rest) > 0; {
		line := rest

		if end := bytes.IndexByte(rest, '\n'); end >= 0 {
			line, rest = rest[:end], rest[end+1:]
		} else {
			rest = nil
		}

		width := 0
		for width < len(line) && (line[width] == ' ' || line[width] == '\t') {
			width++
		}

		if width == 0 {
			continue
		}

		content := bytes.TrimSuffix(line[width:], []byte("\r"))

		// A line of nothing but whitespace says nothing about indentation.
		if len(content) == 0 {
			continue
		}

		// Comments keep their own text rather than being re-indented. Treating
		// their opening, continuation or closing line as a document level would
		// let a banner change the indentation of every value on save.
		if content[0] == '/' || content[0] == '*' {
			continue
		}

		return string(line[:width]), true
	}

	return "", false
}
