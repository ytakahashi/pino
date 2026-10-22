package picker

// DirEntry is one name in a directory, as the file system reports it.
//
// It is a struct rather than a bare name so that what the picker learns about
// an entry can grow — a modification time to sort by, say — by adding a field
// instead of changing the port and every implementation of it.
type DirEntry struct {
	Name string
	Dir  bool
}

// DirLister reads the names in a directory.
//
// It reports regular files and directories only, with a symbolic link
// reported as what it points to; anything else cannot be opened as a
// document and is left out. Which of them are candidates is decided here,
// not by the lister.
//
// It is declared in this package rather than among the ports of application
// because the picker does not import application: the two are kept apart so
// that neither can reach into the other.
type DirLister interface {
	List(dir string) ([]DirEntry, error)
}
