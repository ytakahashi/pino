package cli

import "testing"

func programConfigFor(t *testing.T, args ...string) ProgramConfig {
	t.Helper()

	parser := newOptionParser()
	if err := parser.parse(args); err != nil {
		t.Fatalf("parse(%v): %v", args, err)
	}

	config, err := configFrom(parser.options)
	if err != nil {
		t.Fatalf("configFrom(%v): %v", args, err)
	}

	return config
}
