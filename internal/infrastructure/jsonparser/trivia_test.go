package jsonparser

import (
	"errors"
	"iter"
	"testing"

	"github.com/tailscale/hujson"

	"github.com/ytakahashi/pino/internal/domain"
)

func TestParseAssignsCommentsToTheirDocumentPositions(t *testing.T) {
	src := `{
  // heading
  "a" /* key */ : /* value */ 1 /* before comma */, /* inline gap */
  // next member
  "b": 2 // trailing
  // inside
}`

	node, err := New().Parse([]byte(src), domain.JSONC)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	object, ok := node.(*domain.Object)
	if !ok {
		t.Fatalf("parsed %T, want *domain.Object", node)
	}

	first := object.At(0)
	wantComments(t, first.Trivia.Before(), []commentWant{{" heading", false, true}})
	wantComments(t, first.Value.Trivia().Before(), []commentWant{
		{" key ", true, false},
		{" value ", true, false},
	})
	wantComments(t, first.Value.Trivia().After(), nil)
	wantComments(t, first.Trivia.After(), []commentWant{
		{" before comma ", true, false},
		{" inline gap ", true, false},
	})

	second := object.At(1)
	wantComments(t, second.Trivia.Before(), []commentWant{{" next member", false, true}})
	wantComments(t, second.Trivia.After(), []commentWant{{" trailing", false, false}})
	wantComments(t, object.Trivia().Inside(), []commentWant{{" inside", false, true}})
}

func TestParseAssignsCommentsAroundArraysAndTheRoot(t *testing.T) {
	src := `// before root
[
  /* first */ 1, /* second follows */
  2,
  // inside
]
// after root
`

	node, err := New().Parse([]byte(src), domain.JSONC)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	wantComments(t, node.Trivia().Before(), []commentWant{{" before root", false, true}})
	wantComments(t, node.Trivia().After(), []commentWant{{" after root", false, true}})

	array, ok := node.(*domain.Array)
	if !ok {
		t.Fatalf("parsed %T, want *domain.Array", node)
	}
	wantComments(t, array.At(0).Trivia().Before(), []commentWant{{" first ", true, true}})
	wantComments(t, array.At(0).Trivia().After(), []commentWant{{" second follows ", true, false}})
	wantComments(t, array.Trivia().Inside(), []commentWant{{" inside", false, true}})
}

func TestParseCarriesNewlineStateAcrossPunctuation(t *testing.T) {
	t.Run("before a middle comma", func(t *testing.T) {
		node, err := New().Parse([]byte("[1\n// next\n,\n2]"), domain.JSONC)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}

		array := node.(*domain.Array)
		wantComments(t, array.At(0).Trivia().After(), nil)
		wantComments(t, array.At(1).Trivia().Before(), []commentWant{{" next", false, true}})
	})

	t.Run("across a colon preceded by a newline", func(t *testing.T) {
		node, err := New().Parse([]byte("{\"a\" // key\n: /* value */ 1}"), domain.JSONC)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}

		object := node.(*domain.Object)
		wantComments(t, object.At(0).Value.Trivia().Before(), []commentWant{
			{" key", false, false},
			{" value ", true, true},
		})
	})

	t.Run("after a comma preceded by a newline", func(t *testing.T) {
		node, err := New().Parse([]byte("[1\n, /* next */ 2]"), domain.JSONC)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}

		array := node.(*domain.Array)
		wantComments(t, array.At(0).Trivia().After(), nil)
		wantComments(t, array.At(1).Trivia().Before(), []commentWant{{" next ", true, true}})
	})

	t.Run("before a closing comma", func(t *testing.T) {
		node, err := New().Parse([]byte("{\"a\":1\n// inside\n,}"), domain.JSONC)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}

		object := node.(*domain.Object)
		wantComments(t, object.At(0).Trivia.After(), nil)
		wantComments(t, object.Trivia().Inside(), []commentWant{{" inside", false, true}})
	})
}

func TestParseCopiesCommentTextFromTheSource(t *testing.T) {
	src := []byte("// note\n1")
	node, err := New().Parse(src, domain.JSONC)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	before := string(domain.Encode(node, domain.DefaultFormat()))
	for i := range src {
		src[i] = 'x'
	}

	if got := string(domain.Encode(node, domain.DefaultFormat())); got != before {
		t.Errorf("tree changed with the source: %q, was %q", got, before)
	}
}

func TestParseReportsInvalidUTF8InAComment(t *testing.T) {
	_, err := New().Parse([]byte("// \xff\n1"), domain.JSONC)
	if err == nil {
		t.Fatal("Parse succeeded, want an error")
	}

	var syntaxErr *SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("error is %T, want *SyntaxError", err)
	}
	wantPosition(t, syntaxErr, 1, 4)

	var invalidComment *domain.InvalidCommentError
	if !errors.As(err, &invalidComment) {
		t.Fatalf("error does not unwrap to *domain.InvalidCommentError: %v", err)
	}

	var invalidUTF8 *domain.InvalidUTF8Error
	if !errors.As(err, &invalidUTF8) {
		t.Fatalf("error does not unwrap to *domain.InvalidUTF8Error: %v", err)
	}
}

func TestParseReportsAnEarlierSyntaxErrorBeforeInvalidCommentText(t *testing.T) {
	err := syntaxErrorFor(t, "{]\n// \xff\n", domain.JSONC)
	wantPosition(t, err, 1, 2)

	var invalidComment *domain.InvalidCommentError
	if errors.As(err, &invalidComment) {
		t.Errorf("error unwraps to a later invalid comment: %v", err)
	}
}

func TestCommentScannerRejectsMalformedExtra(t *testing.T) {
	tests := map[string]hujson.Extra{
		"incomplete opener":      hujson.Extra("/"),
		"unterminated line":      hujson.Extra("// comment"),
		"unterminated block":     hujson.Extra("/* comment"),
		"empty unclosed block":   hujson.Extra("/*"),
		"slash before non-slash": hujson.Extra("/x"),
	}

	for name, extra := range tests {
		t.Run(name, func(t *testing.T) {
			scanner := commentScanner{tokenOnLine: true}
			if _, err := scanner.comments(extra, 0, extra); err == nil {
				t.Errorf("comments(%q) succeeded, want an error", extra)
			}
		})
	}
}

type commentWant struct {
	text    string
	block   bool
	ownLine bool
}

func wantComments(t *testing.T, seq iter.Seq[domain.Comment], want []commentWant) {
	t.Helper()

	var got []commentWant
	seq(func(comment domain.Comment) bool {
		got = append(got, commentWant{comment.Text(), comment.Block(), comment.OwnLine()})
		return true
	})

	if len(got) != len(want) {
		t.Fatalf("comments = %#v, want %#v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("comment %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}
