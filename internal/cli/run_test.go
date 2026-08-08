package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/infrastructure/jsonparser"
)

// run drives Run with its output captured.
//
// None of these cases reach the terminal interface: every one of them is
// answered before a document is open, which is what makes the whole of Run
// testable without a terminal. The one path that does start it is covered by
// the end-to-end test in the presentation layer.
func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	var out, errOut bytes.Buffer

	code = Run(args, &out, &errOut)

	return code, out.String(), errOut.String()
}

// write puts contents in a file of its own and returns the path.
func write(t *testing.T, name, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}

	return path
}

func TestUsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"two files", []string{"a.json", "b.json"}},
		{"unknown flag", []string{"-nope", "a.json"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			code, stdout, stderr := run(t, tt.args...)

			if code != exitUsage {
				t.Errorf("exit code = %d, want %d", code, exitUsage)
			}

			if stdout != "" {
				t.Errorf("stdout = %q, want it empty", stdout)
			}

			if !strings.Contains(stderr, "usage: pino") {
				t.Errorf("stderr = %q, want it to describe the usage", stderr)
			}
		})
	}
}

// An unknown flag is named, so that the usage is an aid rather than the whole
// of the answer.
func TestUnknownFlagIsNamed(t *testing.T) {
	t.Parallel()

	_, _, stderr := run(t, "-nope")

	if !strings.Contains(stderr, "nope") {
		t.Errorf("stderr = %q, want it to name the flag", stderr)
	}
}

// Help was asked for, so it is output rather than a complaint.
func TestHelp(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := run(t, "-help")

	if code != exitOK {
		t.Errorf("exit code = %d, want %d", code, exitOK)
	}

	if !strings.Contains(stdout, "usage: pino") {
		t.Errorf("stdout = %q, want it to describe the usage", stdout)
	}

	for _, flag := range []string{"-version", "-indent"} {
		if !strings.Contains(stdout, flag) {
			t.Errorf("stdout = %q, want it to list %s", stdout, flag)
		}
	}

	if stderr != "" {
		t.Errorf("stderr = %q, want it empty", stderr)
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := run(t, "-version")

	if code != exitOK {
		t.Errorf("exit code = %d, want %d", code, exitOK)
	}

	if !strings.HasPrefix(stdout, "pino ") || !strings.HasSuffix(stdout, "\n") {
		t.Errorf("stdout = %q, want one line naming pino", stdout)
	}

	if stderr != "" {
		t.Errorf("stderr = %q, want it empty", stderr)
	}
}

// The version is answered before the argument is looked at, so that it can be
// asked for on its own.
func TestVersionWithoutAFile(t *testing.T) {
	t.Parallel()

	if code, _, _ := run(t, "-version"); code != exitOK {
		t.Errorf("exit code = %d, want %d", code, exitOK)
	}
}

// A file that cannot be read is reported by the error the file store raised,
// which names the path itself. Nothing is added around it, so the path is not
// printed twice.
func TestUnreadableFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path func(t *testing.T) string
		want string
	}{
		{
			name: "missing",
			path: func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing.json") },
			want: "no such file",
		},
		{
			name: "directory",
			path: func(t *testing.T) string { return t.TempDir() },
			want: "is a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := tt.path(t)

			code, stdout, stderr := run(t, path)

			if code != exitError {
				t.Errorf("exit code = %d, want %d", code, exitError)
			}

			if stdout != "" {
				t.Errorf("stdout = %q, want it empty", stdout)
			}

			if want := "pino: "; !strings.HasPrefix(stderr, want) {
				t.Errorf("stderr = %q, want it to start with %q", stderr, want)
			}

			if !strings.Contains(stderr, tt.want) {
				t.Errorf("stderr = %q, want it to mention %q", stderr, tt.want)
			}

			if strings.Count(stderr, path) != 1 {
				t.Errorf("stderr = %q, want the path in it exactly once", stderr)
			}
		})
	}
}

// Everything that stops a document from being read is reported the same way:
// one line, naming the file and the position within it.
func TestDocumentThatCannotBeOpened(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
		want     string
	}{
		{
			name:     "syntax error",
			contents: "{\n  \"host\" \"localhost\"\n}\n",
			want:     "2:10: invalid character '\"' after object name",
		},
		{
			name:     "comment",
			contents: "{\n  // a note\n  \"host\": \"localhost\"\n}\n",
			want:     "2:3: comments are not supported yet",
		},
		{
			name:     "trailing comma",
			contents: "[\n  1,\n]\n",
			want:     "2:4: trailing commas are not supported yet",
		},
		{
			name:     "duplicate key",
			contents: "{\n  \"a\": 1,\n  \"a\": 2\n}\n",
			want:     "3:3:",
		},
		{
			name:     "invalid UTF-8",
			contents: "{\n  \"a\": \"x\xffy\"\n}\n",
			want:     "2:10: invalid UTF-8 in string",
		},
		{
			name:     "unpaired surrogate",
			contents: "{\n  \"a\": \"\\ud800\"\n}\n",
			want:     "2:9: unpaired surrogate escape in string",
		},
		{
			name:     "empty file",
			contents: "",
			want:     "1:1:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := write(t, "broken.json", tt.contents)

			code, stdout, stderr := run(t, path)

			if code != exitError {
				t.Errorf("exit code = %d, want %d", code, exitError)
			}

			if stdout != "" {
				t.Errorf("stdout = %q, want it empty", stdout)
			}

			if want := "pino: " + path + ":" + tt.want; !strings.HasPrefix(stderr, want) {
				t.Errorf("stderr = %q, want it to start with %q", stderr, want)
			}

			if strings.Count(stderr, "\n") != 1 {
				t.Errorf("stderr = %q, want a single line", stderr)
			}
		})
	}
}

// A failure the parser could not place is still reported, with the file it was
// found in and without a position. The error is reached through a wrapping, as
// it would be by the time it comes back from opening a document.
func TestReportOpenErrorWithoutAPosition(t *testing.T) {
	t.Parallel()

	var b bytes.Buffer

	err := fmt.Errorf("parsing: %w", &jsonparser.SyntaxError{Msg: "a reason in some new shape"})

	reportOpenError(&b, "config.json", err)

	if want := "pino: config.json: a reason in some new shape\n"; b.String() != want {
		t.Errorf("reportOpenError wrote %q, want %q", b.String(), want)
	}
}
