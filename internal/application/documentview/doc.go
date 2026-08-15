// Package documentview turns a document into the rows that stand for it.
//
// It owns both halves of that: the model a drawn row is expressed in, and the
// two renderers that build one. Keeping them together is what lets the layer
// above hold a Renderer without knowing which view it has, and what keeps a
// row a row — spans carrying a Role rather than a colour, so that no terminal
// library is needed to produce one.
//
// It is a package of the application layer rather than of presentation because
// cursor movement, scrolling and keeping the selection across a view switch
// all operate on the rows. It depends on domain and on the standard library,
// and on nothing else; in particular it does not depend on the package that
// holds it, which is what lets the session's own tests draw with the real
// renderers.
package documentview
