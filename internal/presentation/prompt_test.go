package presentation

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ytakahashi/pino/internal/application"
)

// What the band asking a question draws, and how many rows it says it needs.
//
// The two are checked against each other rather than separately: the height is
// worked out before anything is drawn, so a band that drew one row more than it
// asked for would push the status bar off the screen.

func TestPromptRowsCountsTheRowsThatAreDrawn(t *testing.T) {
	t.Parallel()

	withError := confirmPrompt()
	withError.Error = "not a JSON number"

	tests := map[string]struct {
		prompt application.PromptInfo
		input  []string
	}{
		"a value being typed":  {textPrompt(false), []string{"8080"}},
		"a value of two rows":  {textPrompt(true), []string{"first", "second"}},
		"the list of types":    {typePrompt(), nil},
		"a question":           {confirmPrompt(), nil},
		"a refused answer":     {withError, nil},
		"nothing being asked":  {application.PromptInfo{}, nil},
		"a refused value":      {refused(textPrompt(false)), []string{"80x0"}},
		"a refused long value": {refused(textPrompt(true)), []string{"a", "b", "c"}},
		"an error notice":      {noticePrompt(application.NoticeError, "permission denied"), nil},
		"a warning notice":     {noticePrompt(application.NoticeWarning, strings.Repeat("x", 120)), nil},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			want := promptRows(tc.prompt, len(tc.input))
			if got := len(bandRows(tc.prompt, tc.input, 60)); got != want {
				t.Errorf("the band drew %d rows, want the %d it asked for", got, want)
			}
		})
	}
}

func TestTheBandPutsTheTitleBesideWhatIsBeingTyped(t *testing.T) {
	t.Parallel()

	got := bandRows(textPrompt(false), []string{"8080"}, 60)

	if len(got) != 2 {
		t.Fatalf("the band drew %d rows, want a rule and a row", len(got))
	}

	if !strings.HasPrefix(got[0], "─") {
		t.Errorf("the band starts with %q, want a rule dividing it from the document", got[0])
	}

	// The keys the prompt takes are written on the rule, where they cannot
	// take columns from what is being typed.
	if !strings.HasSuffix(got[0], " Enter ok  Esc cancel ") {
		t.Errorf("the rule reads %q, want the keys it takes at the end of it", got[0])
	}

	if got[1] != " Edit number  8080" {
		t.Errorf("the row reads %q, want the title beside the value", got[1])
	}
}

func TestTheBandOffersToMakeANewlineOnlyInAString(t *testing.T) {
	t.Parallel()

	// The hint is the only place the key is written down, so it appears
	// exactly where the key does anything.
	if got := bandRows(textPrompt(true), []string{""}, 80); !strings.Contains(got[0], "Ctrl+j newline") {
		t.Errorf("a string is being edited and the rule reads %q", got[0])
	}

	if got := bandRows(textPrompt(false), []string{""}, 80); strings.Contains(got[0], "Ctrl+j") {
		t.Errorf("a number is being edited and the rule offers a newline: %q", got[0])
	}
}

func TestTheBandIndentsAnAnswerRunningToSeveralRows(t *testing.T) {
	t.Parallel()

	got := bandRows(textPrompt(true), []string{"first", "second"}, 60)

	// Under the first row of the answer rather than under the title: what is
	// being typed reads as one value.
	first := strings.Index(got[1], "first")
	second := strings.Index(got[2], "second")

	if first < 0 || first != second {
		t.Errorf("the answer begins at column %d then %d:\n%q\n%q", first, second, got[1], got[2])
	}
}

func TestTheBandLaysTheChoicesOutInColumns(t *testing.T) {
	t.Parallel()

	got := bandRows(typePrompt(), nil, 60)

	// Every choice is given the width of the longest and a gap, so the columns
	// line up whatever is on offer.
	want := []string{
		" type",
		" [s] string   [n] number   [b] boolean",
	}

	for i, row := range want {
		if got[i+1] != row {
			t.Errorf("row %d is %q, want %q", i+1, got[i+1], row)
		}
	}

	// Three to a row, so the six types take two rows and the second lines up
	// under the first.
	if len(got) != 4 {
		t.Fatalf("the band drew %d rows, want a rule, a title and two rows of choices", len(got))
	}

	if !strings.HasPrefix(got[3], " [z] null     [o] object   [a] array") {
		t.Errorf("the second row of choices is %q, want it aligned under the first", got[3])
	}

	if !strings.HasSuffix(got[0], " Esc cancel ") {
		t.Errorf("the rule is %q, want the way out written on it", got[0])
	}
}

func TestTheBandAsksAQuestionOnARowOfItsOwn(t *testing.T) {
	t.Parallel()

	got := bandRows(confirmPrompt(), nil, 60)

	if got[1] != " Discard 12 child nodes under /server?" {
		t.Errorf("the question reads %q", got[1])
	}

	if !strings.HasPrefix(got[2], " [y] Yes  [n] No") {
		t.Errorf("the answers read %q", got[2])
	}
}

func TestTheBandSaysWhyAnAnswerWasRefusedOnTheLastRow(t *testing.T) {
	t.Parallel()

	p := textPrompt(false)
	p.Error = "not a JSON number"

	got := bandRows(p, []string{"80x0"}, 60)

	if len(got) != 3 {
		t.Fatalf("the band drew %d rows, want a rule, the value and the reason", len(got))
	}

	if got[2] != " not a JSON number" {
		t.Errorf("the last row reads %q, want the reason the answer was refused", got[2])
	}
}

func TestTheBandGivesUpTheHintBeforeTheAnswer(t *testing.T) {
	t.Parallel()

	// The keys go on working whether or not they are written down; what is
	// being typed is the thing being attended to.
	got := bandRows(textPrompt(false), []string{"8080"}, 20)

	if strings.Contains(got[0], "Esc") {
		t.Errorf("the rule reads %q, want the hint left off one with no room", got[0])
	}

	if !strings.Contains(got[1], "8080") {
		t.Errorf("the row reads %q, want what is being typed to survive", got[1])
	}
}

func TestNothingIsDrawnWhenNothingIsAsked(t *testing.T) {
	t.Parallel()

	if got := DefaultTheme().RenderPrompt(application.PromptInfo{}, nil, 60); got != nil {
		t.Errorf("RenderPrompt() = %q, want nothing", got)
	}

	if got := promptRows(application.PromptInfo{}, 0); got != 0 {
		t.Errorf("promptRows() = %d, want 0", got)
	}
}

func TestRuntimeNoticeKeepsItsConclusionAndAcknowledgementOnShortScreens(t *testing.T) {
	t.Parallel()

	notice := application.NoticeInfo{
		Summary:  "Could not save config.json.",
		Detail:   "permission denied because the destination directory has a deliberately long name",
		Severity: application.NoticeError,
	}
	prompt := application.PromptInfo{
		Kind:    application.PromptChoice,
		Choices: []application.Choice{{Key: 'o', Label: "OK"}},
		Notice:  &notice,
	}

	rows := DefaultTheme().RenderPrompt(prompt, nil, 60)
	if got, want := len(rows), ruleRows+noticeBodyRows; got != want {
		t.Fatalf("RenderPrompt() drew %d rows, want %d", got, want)
	}

	plain := bandRows(prompt, nil, 60)
	if !strings.HasSuffix(plain[0], " Esc cancel ") {
		t.Errorf("notice rule = %q, want the cancellation hint", plain[0])
	}

	if !strings.Contains(plain[1], notice.Summary) || !strings.Contains(plain[2], "[o] OK") {
		t.Errorf("notice rows = %q, want conclusion then acknowledgement", plain)
	}

	if !strings.HasSuffix(plain[3], "…") {
		t.Errorf("detail row = %q, want an ellipsis", plain[3])
	}

	for _, row := range rows {
		if got := ansi.StringWidth(row); got > 60 {
			t.Errorf("notice row width = %d, want at most 60: %q", got, row)
		}
	}
}

func TestRuntimeNoticeStylesWarningsDifferentlyFromErrors(t *testing.T) {
	t.Parallel()

	base := application.PromptInfo{Kind: application.PromptChoice}
	errorNotice := application.NoticeInfo{Summary: "Could not save config.json.", Detail: "same detail"}
	warningNotice := application.NoticeInfo{
		Summary: "Saved config.json, but durability could not be confirmed.", Detail: "same detail",
		Severity: application.NoticeWarning,
	}

	base.Notice = &errorNotice
	errorRow := DefaultTheme().RenderPrompt(base, nil, 80)[3]
	base.Notice = &warningNotice
	warningRow := DefaultTheme().RenderPrompt(base, nil, 80)[3]

	if errorRow == warningRow {
		t.Error("warning detail is styled the same as an error detail")
	}
}

func TestRuntimeNoticeDrawsTheChoiceItReceives(t *testing.T) {
	t.Parallel()

	notice := application.NoticeInfo{Summary: "Could not save config.json.", Detail: "permission denied"}
	prompt := application.PromptInfo{
		Kind:    application.PromptChoice,
		Choices: []application.Choice{{Key: 'x', Label: "Close"}},
		Notice:  &notice,
	}

	if got := bandRows(prompt, nil, 60)[2]; got != " [x] Close" {
		t.Errorf("notice choice row = %q, want the choice the prompt offers", got)
	}
}

func TestTheBoxIsToldWhatRoomItHas(t *testing.T) {
	t.Parallel()

	// Enough room for the title, the hint and the gaps around them, and never
	// less than a column: the widget scrolls its own text rather than letting
	// it run under the hint.
	p := textPrompt(false)

	if got := inputWidth(p, 60); got <= 0 || got >= 60 {
		t.Errorf("inputWidth() = %d, want what is left of 60 columns", got)
	}

	if got := inputWidth(p, 2); got != 1 {
		t.Errorf("inputWidth() = %d on a screen with no room, want 1", got)
	}
}
