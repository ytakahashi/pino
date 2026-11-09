package filestore

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ytakahashi/pino/internal/application/picker"
)

// Lister reads directory entries without deciding which names are candidates.
type Lister struct{}

func NewLister() *Lister { return &Lister{} }

var _ picker.DirLister = (*Lister)(nil)

// List reports regular files and directories directly inside dir.
func (l *Lister) List(dir string) ([]picker.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	result := make([]picker.DirEntry, 0, len(entries))
	for _, entry := range entries {
		mode := entry.Type()
		if mode&fs.ModeSymlink != 0 {
			info, err := os.Stat(filepath.Join(dir, entry.Name()))
			if err != nil {
				// An unresolved link cannot be classified; it must not hide
				// the other entries in an otherwise readable directory.
				continue
			}

			mode = info.Mode()
		}

		// In particular, a FIFO must never be reported: opening it blocks
		// until a writer appears.
		if mode.IsRegular() || mode.IsDir() {
			result = append(result, picker.DirEntry{Name: entry.Name(), Dir: mode.IsDir()})
		}
	}

	return result, nil
}
