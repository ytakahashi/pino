package domain

import "testing"

// document is a tree with each of its nodes named, so that a test can assert
// which one came back rather than only what it looks like.
type document struct {
	root     Node
	server   Node
	host     Node
	port     Node
	features Node
	search   Node
	history  Node
	slash    Node
	numeric  Node
}

func newDocument(t *testing.T) document {
	t.Helper()

	var d document

	d.host = str(t, "localhost")
	d.port = NewNumber("8080")
	d.search = str(t, "search")
	d.history = str(t, "history")
	d.features = NewArray([]Node{d.search, d.history})

	d.server = obj(t,
		Member{Key: "host", Value: d.host},
		Member{Key: "port", Value: d.port},
		Member{Key: "features", Value: d.features},
	)

	// A key holding a solidus and a key that reads as a number: both are legal
	// JSON, and both are where addressing a node by text goes wrong if the
	// token is not taken literally.
	d.slash = NewBool(true)
	d.numeric = NewNull()

	d.root = obj(t,
		Member{Key: "server", Value: d.server},
		Member{Key: "a/b", Value: d.slash},
		Member{Key: "0", Value: d.numeric},
	)

	return d
}
