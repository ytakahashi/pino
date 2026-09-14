package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noteValue and otherValue are long enough for their rows to show them cut
// short, and read differently from each other, so that a block written out
// beneath the wrong member does not pass for the right one.
var (
	noteValue  = strings.Repeat("note ", 30)
	otherValue = strings.Repeat("other ", 25)
)

// writeLongValues puts a document holding two long strings on disk and
// answers the path to it.
func writeLongValues(t *testing.T) string {
	t.Helper()

	document := `{
    "note": "` + noteValue + `",
    "other": "` + otherValue + `",
    "port": 8080
}
`

	path := filepath.Join(t.TempDir(), "long.json")

	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}

	return path
}
