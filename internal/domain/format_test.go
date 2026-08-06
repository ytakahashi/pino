package domain_test

import (
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

func TestDefaultFormat(t *testing.T) {
	t.Parallel()

	got := domain.DefaultFormat()

	if got.Indent != "  " {
		t.Errorf("Indent = %q, want two spaces", got.Indent)
	}

	if got.Newline != "\n" {
		t.Errorf("Newline = %q, want %q", got.Newline, "\n")
	}

	if !got.TrailingNL {
		t.Error("TrailingNL = false, want true")
	}
}

// Empty input carries no layout at all. Concluding "no trailing newline" from
// zero bytes would be an inference, and it would leave a document created from
// scratch without the final newline that DefaultFormat promises.
func TestDetectFormatOnEmptyInputReturnsTheDefault(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		src  []byte
	}{
		{"nil", nil},
		{"empty slice", []byte{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := domain.DetectFormat(tt.src); got != domain.DefaultFormat() {
				t.Errorf("DetectFormat(%s) = %+v, want DefaultFormat() = %+v",
					tt.name, got, domain.DefaultFormat())
			}
		})
	}
}

func TestDetectFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want domain.Format
	}{
		{
			name: "two spaces",
			src:  "{\n  \"host\": \"localhost\"\n}\n",
			want: domain.Format{Indent: "  ", Newline: "\n", TrailingNL: true},
		},
		{
			name: "four spaces",
			src:  "{\n    \"host\": \"localhost\"\n}\n",
			want: domain.Format{Indent: "    ", Newline: "\n", TrailingNL: true},
		},
		{
			name: "tabs",
			src:  "{\n\t\"host\": \"localhost\"\n}\n",
			want: domain.Format{Indent: "\t", Newline: "\n", TrailingNL: true},
		},
		{
			name: "no trailing newline",
			src:  "{\n  \"host\": \"localhost\"\n}",
			want: domain.Format{Indent: "  ", Newline: "\n", TrailingNL: false},
		},
		{
			// A CRLF file rewritten with LF is a whole-file diff, which is
			// exactly what detecting the layout is meant to prevent.
			name: "crlf",
			src:  "{\r\n    \"host\": \"localhost\"\r\n}\r\n",
			want: domain.Format{Indent: "    ", Newline: "\r\n", TrailingNL: true},
		},
		{
			name: "crlf without a trailing newline",
			src:  "{\r\n\t\"host\": \"localhost\"\r\n}",
			want: domain.Format{Indent: "\t", Newline: "\r\n", TrailingNL: false},
		},
		{
			// Nothing here says how to indent or how to end a line, but the
			// missing trailing newline is a fact about the file and is kept.
			name: "single line keeps its missing trailing newline",
			src:  `{"host":"localhost"}`,
			want: domain.Format{Indent: "  ", Newline: "\n", TrailingNL: false},
		},
		{
			name: "single line with a trailing newline",
			src:  "{\"host\":\"localhost\"}\n",
			want: domain.Format{Indent: "  ", Newline: "\n", TrailingNL: true},
		},
		{
			name: "empty object over two lines has nothing indented",
			src:  "{\n}\n",
			want: domain.Format{Indent: "  ", Newline: "\n", TrailingNL: true},
		},
		{
			name: "unindented members fall back to the default",
			src:  "{\n\"host\": \"localhost\"\n}\n",
			want: domain.Format{Indent: "  ", Newline: "\n", TrailingNL: true},
		},
		{
			name: "blank lines are skipped",
			src:  "{\n\n   \n      \"host\": \"localhost\"\n}\n",
			want: domain.Format{Indent: "      ", Newline: "\n", TrailingNL: true},
		},
		{
			name: "whitespace-only lines with a carriage return are skipped",
			src:  "{\r\n   \r\n  \"host\": \"localhost\"\r\n}\r\n",
			want: domain.Format{Indent: "  ", Newline: "\r\n", TrailingNL: true},
		},
		{
			// Indentation is measured from the second line onward, so leading
			// whitespace in front of the root is not mistaken for it.
			name: "whitespace before the root is not indentation",
			src:  "    {\n\t\"host\": \"localhost\"\n}\n",
			want: domain.Format{Indent: "\t", Newline: "\n", TrailingNL: true},
		},
		{
			name: "deeper nesting keeps its own width",
			src:  "{\n  \"server\": {\n    \"host\": \"localhost\"\n  }\n}\n",
			want: domain.Format{Indent: "  ", Newline: "\n", TrailingNL: true},
		},
		{
			name: "array elements count as indentation",
			src:  "[\n   \"search\",\n   \"history\"\n]\n",
			want: domain.Format{Indent: "   ", Newline: "\n", TrailingNL: true},
		},
		{
			name: "a value on its own line counts",
			src:  "{\"host\":\n  \"localhost\"}\n",
			want: domain.Format{Indent: "  ", Newline: "\n", TrailingNL: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := domain.DetectFormat([]byte(tt.src))

			if got != tt.want {
				t.Errorf("DetectFormat(%q) = %+v, want %+v", tt.src, got, tt.want)
			}
		})
	}
}

// Escaped whitespace inside a string is two characters, not a control one, and
// JSON forbids the real thing there. Scanning lines cannot therefore wander
// into a value.
func TestDetectFormatIgnoresEscapesInsideStrings(t *testing.T) {
	t.Parallel()

	src := "{\n\t\"banner\": \"line1\\nline2\\ttabbed\"\n}\n"

	got := domain.DetectFormat([]byte(src))
	want := domain.Format{Indent: "\t", Newline: "\n", TrailingNL: true}

	if got != want {
		t.Errorf("DetectFormat() = %+v, want %+v", got, want)
	}
}

// The first indented line decides, so a document that starts deeper than one
// level reports the wider unit. That changes the layout of a rewrite but not
// its meaning, and this pins the behaviour so a change to it is deliberate.
func TestDetectFormatUsesTheFirstIndentedLine(t *testing.T) {
	t.Parallel()

	src := "{\"server\": {\n        \"host\": \"localhost\"\n}}\n"

	if got := domain.DetectFormat([]byte(src)).Indent; got != "        " {
		t.Errorf("Indent = %q, want eight spaces", got)
	}
}
