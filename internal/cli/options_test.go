package cli

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/ytakahashi/pino/internal/application"
)

// Every option pino takes is one config away from the session it starts, and
// "not given" is one of the answers each of them has. A width of zero asks for
// no indentation while the option being absent asks for the file's own.
func TestCommandOptionsAreReadIntoAProgramConfig(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args []string
		want ProgramConfig
	}{
		"not given": {
			want: ProgramConfig{},
		},
		"none at all": {
			args: []string{"--indent", "0"},
			want: ProgramConfig{Application: application.Config{IndentOverride: "", OverrideIndent: true}},
		},
		"one space with a short option": {
			args: []string{"-i", "1"},
			want: ProgramConfig{Application: application.Config{IndentOverride: " ", OverrideIndent: true}},
		},
		"two spaces with an equals sign": {
			args: []string{"--indent=2"},
			want: ProgramConfig{Application: application.Config{IndentOverride: "  ", OverrideIndent: true}},
		},
		"four spaces joined to a short option": {
			args: []string{"-i4"},
			want: ProgramConfig{Application: application.Config{IndentOverride: "    ", OverrideIndent: true}},
		},
		"the widest there is": {
			args: []string{"-i=" + strconv.Itoa(maxIndent)},
			want: ProgramConfig{Application: application.Config{
				IndentOverride: strings.Repeat(" ", maxIndent), OverrideIndent: true,
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			parser := newOptionParser()
			if err := parser.parse(tc.args); err != nil {
				t.Fatalf("parse(%v): %v", tc.args, err)
			}

			got, err := configFrom(parser.options)
			if err != nil {
				t.Fatalf("configFrom: %v", err)
			}

			if got != tc.want {
				t.Errorf("configFrom(%v) = %#v, want %#v", tc.args, got, tc.want)
			}
		})
	}
}

func TestLongAndShortIndentOptionsCreateTheSameProgramConfig(t *testing.T) {
	t.Parallel()

	long := programConfigFor(t, "--indent", "2")
	short := programConfigFor(t, "-i", "2")

	if short != long {
		t.Errorf("-i config = %#v, want --indent config %#v", short, long)
	}
}

func TestOptionsMayAppearOnEitherSideOfTheFile(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"before": {"--indent", "2", "config.json"},
		"after":  {"config.json", "--indent", "2"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			parser := newOptionParser()
			if err := parser.parse(args); err != nil {
				t.Fatalf("parse(%v): %v", args, err)
			}

			if got, want := parser.options.arguments, []string{"config.json"}; len(got) != 1 || got[0] != want[0] {
				t.Errorf("arguments = %v, want %v", got, want)
			}

			if got := parser.options.indent; got != 2 {
				t.Errorf("indent = %d, want 2", got)
			}
		})
	}
}

func TestDoubleDashAllowsAPathStartingWithAHyphen(t *testing.T) {
	t.Parallel()

	parser := newOptionParser()
	if err := parser.parse([]string{"--", "-config.json"}); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if got, want := parser.options.arguments, []string{"-config.json"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("arguments = %v, want %v", got, want)
	}
}

func TestTheCommandLineSelectsAFileOrStandardInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		terminal  bool
		want      documentChoice
		wantErr   string
		wantUsage bool
	}{
		{name: "file from a terminal", args: []string{"in.json"}, terminal: true, want: documentChoice{path: "in.json"}},
		{name: "file ignores a pipe", args: []string{"in.json"}, want: documentChoice{path: "in.json"}},
		{name: "file rejects output", args: []string{"in.json", "-o", "out.json"}, wantErr: "--output is only used when reading standard input"},
		{name: "explicit stdin", args: []string{"-"}, want: documentChoice{stdin: true}},
		{name: "explicit stdin with output", args: []string{"-", "-o", "out.json"}, want: documentChoice{stdin: true, output: "out.json"}},
		{name: "explicit terminal", args: []string{"-"}, terminal: true, wantErr: "standard input is a terminal"},
		{name: "explicit terminal with output", args: []string{"-", "-o", "out.json"}, terminal: true, wantErr: "standard input is a terminal"},
		{name: "implicit stdin", want: documentChoice{stdin: true}},
		{name: "implicit stdin with output", args: []string{"-o", "out.json"}, want: documentChoice{stdin: true, output: "out.json"}},
		{name: "no input", terminal: true, wantUsage: true},
		{name: "implicit terminal with output", args: []string{"-o", "out.json"}, terminal: true, wantErr: "standard input is a terminal"},
		{name: "several files", args: []string{"a", "b"}, wantUsage: true},
		{name: "several files from a terminal", args: []string{"a", "b"}, terminal: true, wantUsage: true},
		{name: "empty output", args: []string{"-o", ""}, wantErr: "--output needs a path"},
		{name: "stdout destination", args: []string{"-o", "-"}, wantErr: "--output cannot be standard output"},
		{name: "literal dash file", args: []string{"./-"}, terminal: true, want: documentChoice{path: "./-"}},
		{name: "dash after separator", args: []string{"--", "-"}, want: documentChoice{stdin: true}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parser := newOptionParser()
			if err := parser.parse(tc.args); err != nil {
				t.Fatalf("parse: %v", err)
			}

			got, err := documentFrom(parser.options, tc.terminal)
			if tc.wantUsage {
				if !errors.Is(err, errUsageOnly) {
					t.Fatalf("documentFrom = %v, want usage only", err)
				}

				return
			}

			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("documentFrom = %v, want %q", err, tc.wantErr)
				}

				return
			}

			if err != nil || got != tc.want {
				t.Errorf("documentFrom = %+v, %v, want %+v", got, err, tc.want)
			}
		})
	}
}

func TestOutputSpellingsSelectTheSameStdinDestination(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"--output", "out.json", "-"},
		{"-", "--output=out.json"},
		{"-o", "out.json", "-"},
		{"-", "-o=out.json"},
		{"-oout.json", "-"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()

			parser := newOptionParser()
			if err := parser.parse(args); err != nil {
				t.Fatalf("parse: %v", err)
			}

			want := documentChoice{stdin: true, output: "out.json"}
			if got, err := documentFrom(parser.options, false); err != nil || got != want {
				t.Errorf("documentFrom = %+v, %v, want %+v", got, err, want)
			}
		})
	}
}
