// Package e2e drives pino the way the command line assembles it: a real file
// on disk, the real adapters, the real renderers and a real Bubble Tea
// program, with nothing doubled.
//
// It is the one package allowed to name every layer at once, which is why it
// carries a rule of its own in .golangci.yml rather than being left without
// one. What it says is that the wiring holds together — that the bytes a file
// store reads are the ones a parser is given, that the tree it returns is what
// the renderers draw, and that keys reach the session that answers them. What
// any one of those pieces does is settled where that piece is tested, so the
// scenarios here are few and each is short.
package e2e
