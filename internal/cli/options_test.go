package cli

import (
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
