package domain

import "testing"

// Trivia travels inside Member, which is handed out by value. Copying the
// Member copies the slice headers but not their backing arrays, so Trivia has
// to be immutable in its own right for a handed-out Member to be harmless.
func TestNewTriviaCopiesItsInput(t *testing.T) {
	t.Parallel()

	before := []Comment{{Text: "above"}}
	after := []Comment{{Text: "trailing", Block: true}}

	tv := NewTrivia(before, after)

	before[0].Text = "hijacked"
	after[0].Text = "hijacked"

	var gotBefore []Comment
	for c := range tv.Before() {
		gotBefore = append(gotBefore, c)
	}

	if len(gotBefore) != 1 || gotBefore[0].Text != "above" {
		t.Errorf("Before() = %+v, want one comment %q", gotBefore, "above")
	}

	var gotAfter []Comment
	for c := range tv.After() {
		gotAfter = append(gotAfter, c)
	}

	if len(gotAfter) != 1 || gotAfter[0].Text != "trailing" || !gotAfter[0].Block {
		t.Errorf("After() = %+v, want one block comment %q", gotAfter, "trailing")
	}

	if tv.IsEmpty() {
		t.Error("IsEmpty() = true for a Trivia holding comments")
	}

	if !NewTrivia(nil, nil).IsEmpty() {
		t.Error("IsEmpty() = false for a Trivia with no comments")
	}
}
