package filestore

// lookalike has the shape of meta without being it, standing in for a Meta
// issued by some other store.
type lookalike struct {
	hash [32]byte
}
