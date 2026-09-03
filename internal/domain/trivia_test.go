package domain

import (
	"errors"
	"slices"
	"testing"
)

func TestNewCommentBuildsValidatedValue(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		text           string
		block, ownLine bool
	}{
		"a line comment":       {text: " note"},
		"an inline block":      {text: " why ", block: true},
		"an own-line block":    {text: " banner\n body ", block: true, ownLine: true},
		"the replacement rune": {text: " �", ownLine: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := NewComment(tt.text, tt.block, tt.ownLine)
			if err != nil {
				t.Fatalf("NewComment: %v", err)
			}

			if got.Text() != tt.text || got.Block() != tt.block || got.OwnLine() != tt.ownLine {
				t.Errorf("NewComment = (%q, %t, %t), want (%q, %t, %t)",
					got.Text(), got.Block(), got.OwnLine(), tt.text, tt.block, tt.ownLine)
			}
		})
	}
}

func TestNewCommentRejectsTextThatCouldEscapeItsDelimiter(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		text   string
		block  bool
		reason string
	}{
		"line feed in a line comment": {
			text: " first\nsecond", reason: "line comment contains a newline",
		},
		"carriage return in a line comment": {
			text: " first\rsecond", reason: "line comment contains a newline",
		},
		"a block terminator": {
			text: " first */ second ", block: true, reason: "block comment contains */",
		},
		"invalid UTF-8": {
			text: " note \xff", reason: "invalid UTF-8",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := NewComment(tt.text, tt.block, false)
			if err == nil {
				t.Fatal("NewComment succeeded, want an error")
			}

			var invalid *InvalidCommentError
			if !errors.As(err, &invalid) {
				t.Fatalf("error = %T, want *InvalidCommentError", err)
			}

			if invalid.Text != tt.text || invalid.Reason != tt.reason {
				t.Errorf("error = (%q, %q), want (%q, %q)",
					invalid.Text, invalid.Reason, tt.text, tt.reason)
			}
		})
	}
}

func TestInvalidCommentKeepsTheInvalidUTF8Position(t *testing.T) {
	t.Parallel()

	_, err := NewComment(" note \xff", false, false)
	if err == nil {
		t.Fatal("NewComment succeeded, want an error")
	}

	var invalid *InvalidUTF8Error
	if !errors.As(err, &invalid) {
		t.Fatalf("error chain has no *InvalidUTF8Error: %v", err)
	}

	if invalid.Text != " note \xff" || invalid.Index != 6 {
		t.Errorf("InvalidUTF8Error = (%q, %d), want (%q, %d)",
			invalid.Text, invalid.Index, " note \xff", 6)
	}
}

// Trivia travels inside Member, which is handed out by value. Copying the
// Member copies the slice headers but not their backing arrays, so Trivia has
// to be immutable in its own right for a handed-out Member to be harmless.
func TestNewTriviaCopiesItsInput(t *testing.T) {
	t.Parallel()

	before := []Comment{comment(t, "above", false, true)}
	after := []Comment{comment(t, "trailing", true, false)}
	inside := []Comment{comment(t, "inside", false, true)}

	tv := NewTrivia(before, after, inside)

	before[0] = comment(t, "hijacked", false, true)
	after[0] = comment(t, "hijacked", true, false)
	inside[0] = comment(t, "hijacked", false, true)

	var gotBefore []Comment
	for c := range tv.Before() {
		gotBefore = append(gotBefore, c)
	}

	if len(gotBefore) != 1 || gotBefore[0].Text() != "above" {
		t.Errorf("Before() = %+v, want one comment %q", gotBefore, "above")
	}

	var gotAfter []Comment
	for c := range tv.After() {
		gotAfter = append(gotAfter, c)
	}

	if len(gotAfter) != 1 || gotAfter[0].Text() != "trailing" || !gotAfter[0].Block() {
		t.Errorf("After() = %+v, want one block comment %q", gotAfter, "trailing")
	}

	gotInside := slices.Collect(tv.Inside())
	if len(gotInside) != 1 || gotInside[0].Text() != "inside" {
		t.Errorf("Inside() = %+v, want one comment %q", gotInside, "inside")
	}

	if tv.IsEmpty() {
		t.Error("IsEmpty() = true for a Trivia holding comments")
	}
	if !tv.HasInside() {
		t.Error("HasInside() = false for a Trivia holding an inside comment")
	}

	empty := NewTrivia(nil, nil, nil)
	if !empty.IsEmpty() {
		t.Error("IsEmpty() = false for a Trivia with no comments")
	}
	if empty.HasInside() {
		t.Error("HasInside() = true for a Trivia with no inside comments")
	}
}
