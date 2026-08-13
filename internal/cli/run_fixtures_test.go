package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// run drives Run with its output captured.
//
// None of these cases reach the terminal interface: every one of them is
// answered before a document is open, which is what makes the whole of Run
// testable without a terminal. The one path that does start it is covered by
// the tests in internal/e2e.
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
