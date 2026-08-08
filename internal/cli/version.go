package cli

import (
	"runtime/debug"
	"strings"
)

// revisionLen is how much of a commit hash is shown. Twelve digits is what git
// itself grows a short hash to on large repositories, and is long enough to
// name a commit unambiguously.
const revisionLen = 12

// version describes the running binary in one line.
//
// It is read from what the Go toolchain records at build time rather than
// stamped in with linker flags, so that the build commands stay ordinary and
// the same version is reported however pino was installed. A binary built from
// a checkout reports the commit it came from and whether that checkout had
// uncommitted changes; one installed from a module path reports the tag.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "pino unknown"
	}

	var b strings.Builder

	b.WriteString("pino ")

	if info.Main.Version == "" {
		b.WriteString("unknown")
	} else {
		b.WriteString(info.Main.Version)
	}

	var revision, modified string

	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value

		case "vcs.modified":
			if s.Value == "true" {
				modified = ", modified"
			}
		}
	}

	// Absent when the build was made outside a repository, or from a module in
	// the cache, where the version alone already says what was built.
	if revision != "" {
		b.WriteString(" (")
		b.WriteString(revision[:min(len(revision), revisionLen)])
		b.WriteString(modified)
		b.WriteString(")")
	}

	return b.String()
}
