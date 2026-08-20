package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/infrastructure/jsonparser"
)

func TestRunReportsUsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"two files", []string{"a.json", "b.json"}},
		{"unknown short option", []string{"-nope", "a.json"}},
		{"unknown long option", []string{"--nope", "a.json"}},
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

	for _, arg := range []string{"-nope", "--nope"} {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()

			_, _, stderr := run(t, arg)

			if !strings.Contains(stderr, "nope") {
				t.Errorf("stderr = %q, want it to name the option", stderr)
			}
		})
	}
}

// Help was asked for, so it is output rather than a complaint.
func TestHelpPrintsUsage(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{"--help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()

			code, stdout, stderr := run(t, arg)

			if code != exitOK {
				t.Errorf("exit code = %d, want %d", code, exitOK)
			}

			if !strings.Contains(stdout, "usage: pino") {
				t.Errorf("stdout = %q, want it to describe the usage", stdout)
			}

			for _, option := range []string{"-h, --help", "-i, --indent int", "-v, --version"} {
				if !strings.Contains(stdout, option) {
					t.Errorf("stdout = %q, want it to list %s", stdout, option)
				}
			}

			if stderr != "" {
				t.Errorf("stderr = %q, want it empty", stderr)
			}
		})
	}
}

func TestLongAndShortVersionOptionsPrintTheSameBuildVersion(t *testing.T) {
	t.Parallel()

	longCode, longStdout, longStderr := run(t, "--version")
	shortCode, shortStdout, shortStderr := run(t, "-v")

	if shortCode != longCode {
		t.Errorf("-v exit code = %d, want --version exit code %d", shortCode, longCode)
	}

	if shortStdout != longStdout {
		t.Errorf("-v stdout = %q, want --version stdout %q", shortStdout, longStdout)
	}

	if shortStderr != longStderr {
		t.Errorf("-v stderr = %q, want --version stderr %q", shortStderr, longStderr)
	}

	if longCode != exitOK {
		t.Errorf("exit code = %d, want %d", longCode, exitOK)
	}

	if !strings.HasPrefix(longStdout, "pino ") || !strings.HasSuffix(longStdout, "\n") {
		t.Errorf("stdout = %q, want one line naming pino", longStdout)
	}

	if longStderr != "" {
		t.Errorf("stderr = %q, want it empty", longStderr)
	}
}

// The version is answered before the argument is looked at, so that it can be
// asked for on its own.
func TestVersionNeedsNoFile(t *testing.T) {
	t.Parallel()

	if code, _, _ := run(t, "--version"); code != exitOK {
		t.Errorf("exit code = %d, want %d", code, exitOK)
	}
}

func TestRunRefusesUnsupportedOptionSpellings(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"single-dash help":    {"-help"},
		"single-dash version": {"-version"},
		"single-dash indent":  {"-indent", "2"},
		"long short help":     {"--h"},
		"long short version":  {"--v"},
		"long short indent":   {"--i", "2"},

		// pflag records h before finding the invalid x. Keep this case to
		// ensure Run gives the parse error precedence over the help request.
		"help with unknown shorthand": {"-hx"},
	}

	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			code, stdout, stderr := run(t, args...)

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

// A file that cannot be read is reported by the error the file store raised,
// which names the path itself. Nothing is added around it, so the path is not
// printed twice.
func TestRunNamesAnUnreadableFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path func(t *testing.T) string
		want string
	}{
		{
			name: "directory",
			path: func(t *testing.T) string { return t.TempDir() },
			want: "is a directory",
		},
		{
			name: "a link to nothing",
			path: func(t *testing.T) string {
				t.Helper()

				dir := t.TempDir()
				link := filepath.Join(dir, "link.json")

				if err := os.Symlink(filepath.Join(dir, "gone.json"), link); err != nil {
					t.Fatalf("Symlink: %v", err)
				}

				return link
			},
			want: "broken symbolic link",
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

// A path holding nothing is not a failure to report: it is where a new
// document starts. Nothing is written until the reader saves, so opening one
// must leave the directory as it found it.
func TestOpeningAPathThatHoldsNothingStartsADocument(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "new.json")

	model, err := NewProgramModel(path, ProgramConfig{})
	if err != nil {
		t.Fatalf("NewProgramModel: %v", err)
	}

	if model == nil {
		t.Fatal("NewProgramModel returned no model and no error")
	}

	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat after opening a missing path returned %v, want it still missing", err)
	}
}

// A width that is not a width is a misuse of the command line, which is a
// different thing from a file that could not be opened, and says so with the
// exit code.
func TestRunRefusesAnIndentThatIsNotAWidth(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"negative":     {"--indent", "-2"},
		"not a number": {"--indent", "wide"},
		"fractional":   {"--indent", "2.5"},

		// A width no document could use is a mistyped one. The largest of
		// them is a string that cannot be built at all, so it has to be
		// refused rather than repeated into.
		"wider than pino writes": {"--indent", strconv.Itoa(maxIndent + 1)},
		"absurd":                 {"--indent", "1000000"},
		"the largest there is":   {"--indent", strconv.FormatInt(math.MaxInt64, 10)},
	}

	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := write(t, "config.json", "{}\n")

			code, stdout, stderr := run(t, append(args, path)...)

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

// Everything that stops a document from being read is reported the same way:
// one line, naming the file and the position within it.
func TestRunReportsADocumentThatCannotBeOpened(t *testing.T) {
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
func TestReportOpenErrorOmitsAnUnknownPosition(t *testing.T) {
	t.Parallel()

	var b bytes.Buffer

	err := fmt.Errorf("parsing: %w", &jsonparser.SyntaxError{Msg: "a reason in some new shape"})

	reportOpenError(&b, "config.json", err)

	if want := "pino: config.json: a reason in some new shape\n"; b.String() != want {
		t.Errorf("reportOpenError wrote %q, want %q", b.String(), want)
	}
}
